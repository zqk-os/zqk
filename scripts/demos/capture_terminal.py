#!/usr/bin/env python3
"""
Terminal Session Capture & High-Fidelity SVG/PNG Screenshot Generator
Captures terminal output from demo runs and converts it to a styled SVG terminal window.
Ensures 100% XML 1.0 validation conformance (zero unescaped control codes or invalid PCDATA).
Also supports macOS native screencapture when running in desktop environments.
"""

import argparse
import html
import os
import pty
import re
import select
import subprocess
import sys
import xml.etree.ElementTree as ET

# ANSI SGR color mapping (Catppuccin Mocha / Modern Dark palette)
COLOR_MAP = {
    30: "#45475a",  # black
    31: "#f38ba8",  # red
    32: "#a6e3a1",  # green
    33: "#f9e2af",  # yellow
    34: "#89b4fa",  # blue
    35: "#cba6f7",  # magenta
    36: "#94e2d5",  # cyan
    37: "#cdd6f4",  # white
    90: "#585b70",  # bright black
    91: "#eba0ac",  # bright red
    92: "#a6e3a1",  # bright green
    93: "#f9e2af",  # bright yellow
    94: "#89dceb",  # bright blue
    95: "#f5c2e7",  # bright magenta
    96: "#94e2d5",  # bright cyan
    97: "#ffffff",  # bright white
}

# Regex for stripping non-SGR terminal sequences (CSI commands, clear screen, cursor controls, OSC titles)
NON_SGR_REGEX = re.compile(r'\x1b(\[[0-9;?]*[A-Za-ln-z]|\][^\x07\x1b]*(\x07|\x1b\\)|[()][AB012]|[@-Z\\-_])')

# Regex for matching SGR color/style sequences
ANSI_SGR_REGEX = re.compile(r'\x1b\[([0-9;]*)m')


def clean_xml_chars(text: str) -> str:
    """Filters out any characters that are illegal in XML 1.0 documents (e.g. byte 27 ESC)."""
    return "".join(
        c for c in text
        if c in ("\t", "\n", "\r")
        or (0x20 <= ord(c) <= 0xD7FF)
        or (0xE000 <= ord(c) <= 0xFFFD)
        or (0x10000 <= ord(c) <= 0x10FFFF)
    )


def strip_ansi(text: str) -> str:
    text = NON_SGR_REGEX.sub('', text)
    text = ANSI_SGR_REGEX.sub('', text)
    return clean_xml_chars(text)


def ansi_to_svg_spans(line: str) -> str:
    """Converts a line of ANSI-colored text into strict XML-valid SVG tspans."""
    # 1. Strip all non-SGR sequences (clearing screen, cursor moves, OSC)
    line = NON_SGR_REGEX.sub('', line)

    result = []
    current_color = None
    is_bold = False
    is_dim = False

    last_idx = 0
    for match in ANSI_SGR_REGEX.finditer(line):
        text_chunk = line[last_idx:match.start()]
        if text_chunk:
            # Escape HTML entities, then strip any characters illegal in XML
            escaped = clean_xml_chars(html.escape(text_chunk))
            attrs = []
            if current_color:
                attrs.append(f'fill="{current_color}"')
            if is_bold:
                attrs.append('font-weight="bold"')
            if is_dim:
                attrs.append('opacity="0.65"')

            if attrs:
                attr_str = " ".join(attrs)
                result.append(f'<tspan {attr_str}>{escaped}</tspan>')
            else:
                result.append(escaped)

        codes = match.group(1).split(';') if match.group(1) else ['0']
        for c in codes:
            if not c or c == '0':
                current_color = None
                is_bold = False
                is_dim = False
            elif c == '1':
                is_bold = True
            elif c == '2':
                is_dim = True
            elif c == '22':
                is_bold = False
                is_dim = False
            elif c.isdigit():
                code_int = int(c)
                if code_int in COLOR_MAP:
                    current_color = COLOR_MAP[code_int]
                elif code_int == 39:
                    current_color = None

        last_idx = match.end()

    # Remaining text chunk
    text_chunk = line[last_idx:]
    if text_chunk:
        escaped = clean_xml_chars(html.escape(text_chunk))
        attrs = []
        if current_color:
            attrs.append(f'fill="{current_color}"')
        if is_bold:
            attrs.append('font-weight="bold"')
        if is_dim:
            attrs.append('opacity="0.65"')
        if attrs:
            attr_str = " ".join(attrs)
            result.append(f'<tspan {attr_str}>{escaped}</tspan>')
        else:
            result.append(escaped)

    return ''.join(result)


def render_svg(lines: list, title: str = "zqk terminal", width: int = 960) -> str:
    line_height = 20
    top_bar_height = 42
    padding_x = 24
    padding_y = 20

    # Limit displayed lines if too long to prevent huge images
    max_lines = 85
    if len(lines) > max_lines:
        displayed_lines = lines[:max_lines]
        displayed_lines.append("\x1b[2m... [terminal session output truncated for screenshot] ...\x1b[0m")
    else:
        displayed_lines = lines

    content_height = len(displayed_lines) * line_height
    total_height = top_bar_height + padding_y * 2 + content_height

    svg_parts = [
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {total_height}" width="{width}" height="{total_height}">',
        '  <defs>',
        '    <style>',
        '      .window-bg { fill: #1e1e2e; rx: 12px; }',
        '      .top-bar { fill: #181825; }',
        '      .dot-red { fill: #f38ba8; }',
        '      .dot-yellow { fill: #f9e2af; }',
        '      .dot-green { fill: #a6e3a1; }',
        '      .term-text { font-family: "JetBrains Mono", "Fira Code", "Menlo", "Monaco", "Consolas", monospace;',
        '                   font-size: 13px; fill: #cdd6f4; white-space: pre; }',
        '      .title-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;',
        '                    font-size: 12px; fill: #a6adc8; font-weight: 500; text-anchor: middle; }',
        '    </style>',
        '    <filter id="shadow" x="-5%" y="-5%" width="110%" height="110%">',
        '      <feDropShadow dx="0" dy="8" stdDeviation="16" flood-color="#000000" flood-opacity="0.45"/>',
        '    </filter>',
        '  </defs>',
        '',
        '  <!-- Terminal Window Background -->',
        f'  <rect x="4" y="4" width="{width - 8}" height="{total_height - 8}" class="window-bg" filter="url(#shadow)" stroke="#313244" stroke-width="1"/>',
        '',
        '  <!-- Title Bar -->',
        f'  <path d="M 4 16 A 12 12 0 0 1 16 4 L {width - 16} 4 A 12 12 0 0 1 {width - 4} 16 L {width - 4} {top_bar_height} L 4 {top_bar_height} Z" class="top-bar"/>',
        f'  <line x1="4" y1="{top_bar_height}" x2="{width - 4}" y2="{top_bar_height}" stroke="#313244" stroke-width="1"/>',
        '',
        '  <!-- Window Buttons -->',
        '  <circle cx="24" cy="23" r="6" class="dot-red"/>',
        '  <circle cx="44" cy="23" r="6" class="dot-yellow"/>',
        '  <circle cx="64" cy="23" r="6" class="dot-green"/>',
        '',
        '  <!-- Window Title -->',
        f'  <text x="{width / 2}" y="27" class="title-text">{clean_xml_chars(html.escape(title))}</text>',
        '',
        '  <!-- Terminal Text Content -->',
        f'  <g transform="translate({padding_x}, {top_bar_height + padding_y})">',
        '    <text class="term-text">'
    ]

    for idx, raw_line in enumerate(displayed_lines):
        clean_span = ansi_to_svg_spans(raw_line)
        y_pos = (idx + 1) * line_height - 4
        svg_parts.append(f'      <tspan x="0" y="{y_pos}">{clean_span}</tspan>')

    svg_parts.extend([
        '    </text>',
        '  </g>',
        '</svg>'
    ])

    svg_content = '\n'.join(svg_parts)

    # Strictly validate XML conformance before returning
    try:
        ET.fromstring(svg_content)
    except ET.ParseError as err:
        raise RuntimeError(f"Generated SVG failed XML validation: {err}") from err

    return svg_content


def run_and_capture(cmd: list, output_svg: str, title: str, capture_native_png: bool = False, output_png: str = None):
    master_fd, slave_fd = pty.openpty()

    # Start child process in pseudo-terminal
    proc = subprocess.Popen(
        cmd,
        stdin=slave_fd,
        stdout=slave_fd,
        stderr=slave_fd,
        close_fds=True,
        env=dict(os.environ, TERM="xterm-256color", COLUMNS="110", LINES="60")
    )
    os.close(slave_fd)

    captured_chunks = []
    while True:
        try:
            r, _, _ = select.select([master_fd], [], [], 0.1)
            if master_fd in r:
                data = os.read(master_fd, 4096)
                if not data:
                    break
                # Echo live to parent stdout
                sys.stdout.buffer.write(data)
                sys.stdout.buffer.flush()
                captured_chunks.append(data.decode("utf-8", errors="replace"))
        except OSError:
            break

        if proc.poll() is not None:
            # Drain remaining output
            try:
                while True:
                    r, _, _ = select.select([master_fd], [], [], 0.05)
                    if not r:
                        break
                    data = os.read(master_fd, 4096)
                    if not data:
                        break
                    sys.stdout.buffer.write(data)
                    sys.stdout.buffer.flush()
                    captured_chunks.append(data.decode("utf-8", errors="replace"))
            except OSError:
                pass
            break

    os.close(master_fd)
    exit_code = proc.wait()

    # Reconstruct raw output lines
    full_output = ''.join(captured_chunks)
    raw_lines = [line.rstrip() for line in full_output.replace('\r\n', '\n').replace('\r', '\n').split('\n')]
    
    # Filter out initial empty lines
    while raw_lines and not strip_ansi(raw_lines[0]).strip():
        raw_lines.pop(0)

    # Render and validate SVG
    os.makedirs(os.path.dirname(os.path.abspath(output_svg)), exist_ok=True)
    svg_content = render_svg(raw_lines, title=title)
    with open(output_svg, 'w', encoding='utf-8') as f:
        f.write(svg_content)
    print(f"\n\x1b[1;32m✔ Terminal screenshot saved (XML validated):\x1b[0m {output_svg}")

    # If requested and on macOS, also capture native window screenshot
    if capture_native_png and output_png:
        try:
            os.makedirs(os.path.dirname(os.path.abspath(output_png)), exist_ok=True)
            res = subprocess.run(["screencapture", "-x", output_png], capture_output=True)
            if res.returncode == 0:
                print(f"\x1b[1;32m✔ Native desktop screenshot saved:\x1b[0m {output_png}")
        except Exception as e:
            print(f"\x1b[2m(Native screencapture skipped: {e})\x1b[0m")

    return exit_code


def validate_svg_file(path: str) -> bool:
    """Validates an SVG file against the standard XML parser."""
    try:
        ET.parse(path)
        return True
    except ET.ParseError as err:
        print(f"\x1b[1;31m✖ XML error in {path}: {err}\x1b[0m", file=sys.stderr)
        return False


def main():
    parser = argparse.ArgumentParser(description="Capture terminal output and generate styled SVG/PNG screenshots")
    parser.add_argument("--output-svg", "-s", help="Path to output SVG screenshot file")
    parser.add_argument("--output-png", "-p", help="Optional path to output native PNG screenshot file")
    parser.add_argument("--title", "-t", default="zqk terminal", help="Terminal window title")
    parser.add_argument("--capture-native-png", action="store_true", help="Also capture macOS display screenshot via screencapture")
    parser.add_argument("--validate-svg", help="Validate an existing SVG file against XML specification")
    parser.add_argument("command", nargs=argparse.REMAINDER, help="Command to run and capture")

    args = parser.parse_args()

    if args.validate_svg:
        valid = validate_svg_file(args.validate_svg)
        sys.exit(0 if valid else 1)

    if not args.output_svg or not args.command:
        parser.error("Both --output-svg and command are required when running a session capture")

    exit_code = run_and_capture(
        cmd=args.command,
        output_svg=args.output_svg,
        title=args.title,
        capture_native_png=args.capture_native_png,
        output_png=args.output_png
    )
    sys.exit(exit_code)


if __name__ == "__main__":
    main()
