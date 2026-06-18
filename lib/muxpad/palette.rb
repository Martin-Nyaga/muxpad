# frozen_string_literal: true

require "io/console"

module Muxpad
  # An interactive item shown in the palette. +state+ is the human label and
  # +state_kind+ drives its colour (:running, :idle, :finished, :available,
  # :disabled, :unavailable).
  Item = Data.define(:token, :section, :name, :description, :command, :directory, :state, :state_kind)

  # A terminal-native command palette: sectioned, fuzzy-searchable, scrollable.
  # The view layer only renders and reads keys; all launch behaviour lives in
  # Application. #select returns [action, token] (action is "enter", "tab" or
  # "ctrl-r") or nil when cancelled. #choose returns a chosen token or nil.
  class Palette
    # Non-list lines: title, blank, search, blank, blank, command, directory, hint.
    CHROME = 8
    MIN_LIST = 3
    RIGHT_PAD = 2 # blank columns kept clear on the right edge
    NAME_MIN = 10
    NAME_MAX = 24
    RUNNING_SECTION = "RUNNING" # split into the left sidebar rather than the launch list
    SIDEBAR_WIDTH = 20

    RESET = "\e[0m"
    BOLD = "\e[1m"
    DIM = "\e[2m"
    REVERSE = "\e[7m"
    HEADER = "\e[1;35m" # bold magenta
    HIDE_CURSOR = "\e[?25l"
    SHOW_CURSOR = "\e[?25h"

    STATE_COLOR = {
      running: "\e[32m", finished: "\e[33m", available: "\e[32m",
      disabled: DIM, unavailable: "\e[31m", idle: DIM
    }.freeze

    def initialize(input: $stdin, output: $stdout, prompt: "Muxpad")
      @input = input
      @output = output
      @prompt = prompt
    end

    def select(items, section_order:)
      raise Error, "the Muxpad palette requires an interactive terminal" unless @input.tty?
      @running = items.select { |item| item.section == RUNNING_SECTION }
      @launch_items = items.reject { |item| item.section == RUNNING_SECTION }
      @section_order = section_order
      @name_width = name_width(@launch_items)
      @focus = :launch
      @run_cursor = 0
      @query = +""
      refilter
      interact { |key| handle_select_key(key) }
    end

    def choose(options, title:)
      raise Error, "the Muxpad palette requires an interactive terminal" unless @input.tty?
      @choices = options
      @cursor = 0
      render_choice(title)
      @input.raw do
        loop do
          case read_key
          when :up then move_choice(-1)
          when :down then move_choice(1)
          when :enter then return @choices[@cursor]&.first
          when :escape, :cancel then return nil
          else next
          end
          render_choice(title)
        end
      end
    ensure
      teardown
    end

    private

    def interact
      render
      @input.raw do
        loop do
          result = yield read_key
          return result.first if result
          render
        end
      end
    ensure
      teardown
    end

    # Returns nil to keep looping, or a one-element array wrapping the result to
    # return from #select (so that a nil result can itself be returned).
    def handle_select_key(key)
      case key
      when :up then move(-1)
      when :down then move(1)
      when :left then set_focus(:running)
      when :right then set_focus(:launch)
      when :backspace then edit { @query.chop! }
      when :clear then edit { @query.clear }
      when Array then edit { @query << key.last } # [:char, c]
      when :enter then return [selection("enter")] if current
      when :tab then return [selection("tab")] if @focus == :launch && current
      when :restart then return [selection("ctrl-r")] if @focus == :launch && current
      when :escape, :cancel then return [nil]
      end
      nil
    end

    def selection(action)
      [action, current.token]
    end

    # The sidebar only exists while something is running; otherwise the launch
    # list owns the full width and focus stays put.
    def set_focus(pane)
      return if pane == :running && @running.empty?
      @focus = pane
    end

    # Typing always searches the launch list, so editing the query pulls focus
    # back to it from the sidebar.
    def edit
      @focus = :launch
      yield
      refilter
    end

    # Rebuild the grouped, ranked view for the current query and the flat list of
    # rendered rows (headers, blanks, items) the cursor and scroller walk over.
    def refilter
      ranked = @launch_items.filter_map do |item|
        score = score(item)
        [item, score] if score
      end
      grouped = ranked.group_by { |item, _| item.section }
      @rows = []
      @section_order.each do |section|
        entries = grouped[section]
        next unless entries&.any?
        entries = entries.sort_by.with_index { |(_, score), i| [-score, i] }
        @rows << { kind: :blank } unless @rows.empty?
        @rows << { kind: :header, label: section }
        entries.each { |item, _| @rows << { kind: :item, item: } }
      end
      @selectable = @rows.each_index.select { |i| @rows[i][:kind] == :item }
      @cursor = 0
      @offset = 0
    end

    def current
      return @running[@run_cursor] if @focus == :running
      row = @selectable[@cursor]
      row && @rows[row][:item]
    end

    def move(delta)
      if @focus == :running
        return if @running.empty?
        @run_cursor = (@run_cursor + delta).clamp(0, @running.length - 1)
        return
      end
      return if @selectable.empty?
      @cursor = (@cursor + delta).clamp(0, @selectable.length - 1)
      line = @selectable[@cursor]
      height = list_height
      @offset = line if line < @offset
      @offset = line - height + 1 if line > @offset + height - 1
    end

    def move_choice(delta)
      @cursor = (@cursor + delta).clamp(0, @choices.length - 1)
    end

    # --- scoring -------------------------------------------------------------

    # Subsequence match across name/command/description, weighted in that order
    # so a name hit always outranks a command hit, which outranks a description
    # hit. Returns nil when the query matches no field.
    def score(item)
      return 0 if @query.empty?
      query = @query.downcase
      [[item.name, 3], [item.command, 2], [item.description, 1]].filter_map do |text, weight|
        inner = subsequence_score(text.to_s.downcase, query)
        weight * 100_000 + inner if inner
      end.max
    end

    def subsequence_score(text, query)
      first = nil
      from = 0
      query.each_char do |char|
        index = text.index(char, from)
        return nil unless index
        first ||= index
        from = index + 1
      end
      span = from - 1 - first
      [10_000 - first * 10 - span, 0].max
    end

    # --- rendering -----------------------------------------------------------

    def render
      width = columns - RIGHT_PAD
      lines = ["#{BOLD}  #{@prompt}#{RESET}", "", "  #{BOLD}❯#{RESET} #{@query}", ""]
      lines.concat(body_lines(width))
      lines << ""
      lines.concat(detail_lines(width))
      lines << "  #{DIM}#{hint}#{RESET}"
      paint(lines)
      cursor_to(3, 5 + display_width(@query))
    end

    def hint
      switch = sidebar? ? "←/→ switch · " : ""
      "#{switch}enter launch · tab actions · ctrl-r restart · esc close"
    end

    def sidebar? = @running.any?

    # When something is running, the list region is two columns: the running
    # sidebar on the left and the launch list on the right, joined by a divider
    # that runs the height of the taller column.
    def body_lines(width)
      launch = launch_lines(width)
      return launch unless sidebar?

      launch = launch_lines(width - SIDEBAR_WIDTH - 1)
      side = sidebar_lines(SIDEBAR_WIDTH)
      height = [[launch.length, side.length].max, list_height].min
      (0...height).map do |i|
        "#{side[i] || (" " * SIDEBAR_WIDTH)}#{DIM}│#{RESET}#{launch[i]}"
      end
    end

    # Only the rows that actually exist within the viewport — no blank filler, so
    # the detail strip rises to meet a short list instead of floating below a gap.
    def launch_lines(width)
      last = [@offset + list_height, @rows.length].min
      (@offset...last).map { |i| render_row(@rows[i], width) }
    end

    def render_row(row, width)
      case row[:kind]
      when :blank then ""
      when :header then "  #{HEADER}#{row[:label]}#{RESET}"
      when :item then render_item(row[:item], width, @focus == :launch && current.equal?(row[:item]))
      end
    end

    def render_item(item, width, selected)
      avail = width - 2
      state = truncate(item.state.to_s, [avail / 3, 30].min)
      name = truncate(item.name.to_s, @name_width).ljust(@name_width)
      desc_width = [avail - @name_width - 2 - state.length - 1, 0].max
      desc = truncate(item.description.to_s, desc_width).ljust(desc_width)
      if selected
        # Keep the text within the content width, but extend the highlight bar
        # through the reserved right padding so it reaches the popup edge.
        line = "  #{name}  #{desc} #{state}"
        "#{REVERSE}#{line[0, width].ljust(width + RIGHT_PAD)}#{RESET}"
      else
        color = STATE_COLOR.fetch(item.state_kind, DIM)
        "  #{name}  #{DIM}#{desc}#{RESET} #{color}#{state}#{RESET}"
      end
    end

    # The left sidebar: a RUNNING header followed by each live instance, each as
    # a coloured dot and name, padded to the fixed sidebar width.
    def sidebar_lines(width)
      cells = [fill(" #{HEADER}RUNNING#{RESET}", 8, width)]
      @running.each_with_index do |item, i|
        name = truncate(item.name.to_s, width - 3)
        if @focus == :running && i == @run_cursor
          cells << "#{REVERSE}#{" ● #{name}"[0, width].ljust(width)}#{RESET}"
        else
          cells << fill(" #{STATE_COLOR[:running]}●#{RESET} #{name}", 3 + name.length, width)
        end
      end
      cells
    end

    # Pad a string carrying ANSI codes to a visible +width+ using its known
    # printable length, so colour codes don't throw the column alignment off.
    def fill(text, visible, width)
      visible >= width ? text : text + (" " * (width - visible))
    end

    # The two-line preview: the exact command, and the directory it runs in.
    def detail_lines(width)
      item = current
      return ["", ""] unless item
      command = "  #{DIM}$ #{truncate(item.command.to_s, width - 4)}#{RESET}"
      directory = if item.directory.to_s.empty?
        ""
      else
        "  #{DIM}in #{truncate(abbreviate(item.directory), width - 5)}#{RESET}"
      end
      [command, directory]
    end

    def abbreviate(path)
      home = Dir.home
      path.start_with?("#{home}/") ? path.sub(home, "~") : path
    end

    def render_choice(title)
      width = columns - RIGHT_PAD
      lines = ["#{BOLD}  #{title}#{RESET}", ""]
      @choices.each_with_index do |(_, label), i|
        if i == @cursor
          lines << "#{REVERSE}#{"  #{label}".ljust(width)[0, width]}#{RESET}"
        else
          lines << "  #{label}"
        end
      end
      lines << ""
      lines << "  #{DIM}enter select · esc cancel#{RESET}"
      paint(lines)
      cursor_to(lines.length, 1)
    end

    def paint(lines)
      body = lines.map { |line| "#{line}\e[K" }.join("\r\n")
      @output.print("#{HIDE_CURSOR}\e[H#{body}\e[J")
    end

    def cursor_to(row, col)
      @output.print("\e[#{row};#{col}H#{SHOW_CURSOR}")
      @output.flush
    end

    def teardown
      @output.print("#{SHOW_CURSOR}\e[2J\e[H")
      @output.flush
    end

    # --- key input -----------------------------------------------------------

    def read_key
      char = @input.getch
      case char
      when "\r", "\n" then :enter
      when "\t" then :tab
      when "", "\b" then :backspace
      when "" then :cancel
      when "" then :restart
      when "" then :down
      when "" then :up
      when "" then :clear
      when "\e" then escape_key
      else printable(char)
      end
    end

    def escape_key
      return :escape unless @input.wait_readable(0.02) && @input.getch == "["
      case @input.getch
      when "A" then :up
      when "B" then :down
      when "C" then :right
      when "D" then :left
      else :ignore
      end
    end

    def printable(char)
      byte = char&.bytes&.first
      byte && byte >= 0x20 && byte < 0x7F ? [:char, char] : :ignore
    end

    # --- helpers -------------------------------------------------------------

    def name_width(items)
      longest = items.map { |item| item.name.to_s.length }.max.to_i
      longest.clamp(NAME_MIN, NAME_MAX)
    end

    def truncate(value, width)
      return "" if width <= 0
      value.length > width ? "#{value[0, [width - 1, 0].max]}…" : value
    end

    def display_width(value)
      value.length
    end

    def columns
      winsize ? winsize[1] : 80
    end

    # The list fills whatever vertical space the popup gives us (minus chrome),
    # so the window grows with the popup and never leaves a gap below itself.
    def list_height
      rows = winsize ? winsize[0] : 24
      [rows - CHROME, MIN_LIST].max
    end

    def winsize
      (@output.respond_to?(:winsize) && @output.tty? && @output.winsize) || IO.console&.winsize
    end
  end
end
