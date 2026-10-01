#!/usr/bin/env python3
"""Capture a real CLI in a PTY; render its ANSI output for the README.

Capture needs Python on macOS/Linux. Rendering needs Pillow, pyte, and Menlo.
The companion Swift encoder makes an MP4 from the generated PNG frames.
"""
import argparse
import codecs
import errno
import fcntl
import json
import math
import os
from pathlib import Path
import pty
import select
import shlex
import signal
import struct
import sys
import termios
import time


def capture(args):
    command = args.command
    if command and command[0] == "--":
        command = command[1:]
    if not command:
        raise ValueError("supply the real CLI command after --")
    if args.output.exists():
        raise ValueError("refusing to overwrite a recording")
    display = shlex.join([Path(command[0]).name, *command[1:]])
    started, wall_started = time.monotonic(), int(time.time())
    pid, master = pty.fork()
    if pid == 0:
        os.chdir(args.cwd)
        fcntl.ioctl(0, termios.TIOCSWINSZ, struct.pack("HHHH", 32, 96, 0, 0))
        env = dict(os.environ, TERM="xterm-256color", COLUMNS="96", LINES="32")
        env.pop("NO_COLOR", None)
        os.execvpe(command[0], command, env)
    events = [[0, "o", "$ " + display + "\r\n"]]
    decoder = codecs.getincrementaldecoder("utf-8")("replace")
    try:
        while True:
            try:
                ready = select.select([master], [], [], 1)[0]
            except KeyboardInterrupt:
                # Let Tarigato cancel and reap its provider processes normally.
                os.kill(pid, signal.SIGINT)
                continue
            if not ready:
                continue
            try:
                data = os.read(master, 65536)
            except OSError as error:
                if error.errno == errno.EIO:
                    break
                raise
            if not data:
                break
            text = decoder.decode(data)
            if text:
                events.append([round(time.monotonic() - started, 4), "o", text])
                sys.stdout.write(text)
                sys.stdout.flush()
    finally:
        os.close(master)
    _, status = os.waitpid(pid, 0)
    code = os.waitstatus_to_exitcode(status)
    header = {"version": 2, "width": 96, "height": 32,
              "timestamp": wall_started, "title": "Tarigato — session expiry",
              "command": display, "env": {"TERM": "xterm-256color"},
              "tarigato": {"exit_code": code, "elapsed": round(time.monotonic() - started, 3)}}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open("x") as out:
        for item in [header, *events]:
            out.write(json.dumps(item, ensure_ascii=False) + "\n")
    return code


def render(args):
    import pyte
    from PIL import Image, ImageDraw, ImageFont

    lines = [json.loads(line) for line in args.cast.read_text().splitlines()]
    header, events = lines[0], [event for event in lines[1:] if event[1] == "o"]
    if header["tarigato"]["exit_code"] != 0:
        raise ValueError("use a successful run for this README demonstration")
    screen = pyte.Screen(header["width"], header["height"])
    stream = pyte.Stream(screen)
    occupied = 0
    for event in events:
        stream.feed(event[2])
        occupied = max(occupied, max((y + 1 for y, row in enumerate(screen.display) if row.strip()), default=0))
    visible_rows = min(header["height"], occupied + 2)
    screen.reset()
    font_path = str(args.font)
    normal = ImageFont.truetype(font_path, 20)
    bold = ImageFont.truetype(font_path, 20, index=1)
    small = ImageFont.truetype(font_path, 16)
    symbols = ImageFont.truetype("/System/Library/Fonts/Apple Symbols.ttf", 20)
    cell, line_height = 12, 24
    width, height = header["width"] * cell + 112, visible_rows * line_height + 180
    width += width % 2
    height += height % 2
    lead = next((event[0] for event in events if "1 repair max" in event[2]), min(0.25, events[-1][0]))
    speed = max(1, (events[-1][0] - lead) / (args.seconds - 5))
    end = 1 + (events[-1][0] - lead) / speed + 4
    fps = 10
    palette = {"default": "#e1e5ed", "black": "#11121b", "red": "#ef8585",
               "green": "#5fd7af", "brown": "#ffcf87", "blue": "#829fe8",
               "magenta": "#af87ff", "cyan": "#78dce8", "white": "#e1e5ed"}

    def color(value, default):
        if value == "default":
            return default
        return palette.get(value, "#" + value if len(value) == 6 else default)

    base = Image.new("RGB", (width, height), "#080b12")
    draw = ImageDraw.Draw(base)
    # A quiet frame around the captured terminal, not a fabricated interface.
    draw.rounded_rectangle((24, 20, width - 24, height - 66), 18,
                           fill="#11121b", outline="#303044", width=2)
    draw.line((25, 72, width - 25, 72), fill="#2b2c3b", width=1)
    for x, dot in [(49, "#af87ff"), (69, "#5fd7af"), (89, "#ffcf87")]:
        draw.ellipse((x, 42, x + 9, 51), fill=dot)
    draw.text((120, 36), "session-expiry", font=normal, fill="#b7bdcd")
    draw.text((width - 185, 38), "LIVE CODEX", font=small, fill="#9da5ba")
    draw.text((38, height - 42), f"ACTUAL CLI OUTPUT  /  {speed:.1f}x playback", font=small, fill="#9da5ba")
    draw.text((width - 225, height - 42), "TMLS.NYC / TARIGATO", font=small, fill="#af87ff")
    args.frames.mkdir(parents=True, exist_ok=True)
    if any(args.frames.iterdir()):
        raise ValueError("frame directory must be empty")
    frames, event_index, previous, picture = [], 0, None, None
    for number in range(math.ceil(end * fps)):
        now = number / fps
        through = lead + max(0, now - 1) * speed
        while event_index < len(events) and events[event_index][0] <= through:
            stream.feed(events[event_index][2])
            event_index += 1
        rows = [[screen.buffer[y][x] for x in range(screen.columns)] for y in range(visible_rows)]
        state = tuple(tuple(row) for row in rows)
        if state != previous:
            picture = base.copy()
            paint = ImageDraw.Draw(picture)
            for y, row in enumerate(rows):
                x = 0
                while x < len(row):
                    char = row[x]
                    style = (char.fg, char.bg, char.bold, char.reverse)
                    stop = x + 1
                    while stop < len(row) and (row[stop].fg, row[stop].bg, row[stop].bold, row[stop].reverse) == style:
                        stop += 1
                    fg, bg = color(char.fg, "#e1e5ed"), color(char.bg, "#11121b")
                    if char.reverse:
                        fg, bg = bg, fg
                    left, top = 56 + x * cell, 94 + y * line_height
                    if bg != "#11121b":
                        paint.rectangle((left, top, 56 + stop * cell, top + line_height), fill=bg)
                    text = "".join(c.data or " " for c in row[x:stop])
                    if any("\u2800" <= c <= "\u28ff" for c in text):
                        # Pillow has no system font fallback; Menlo lacks Braille.
                        for offset, c in enumerate(text):
                            if "\u2800" <= c <= "\u28ff":
                                paint.text((left + offset * cell + 1, top + 20), c, font=symbols, fill=fg, anchor="ls")
                            else:
                                paint.text((left + offset * cell, top), c, font=normal, fill=fg, anchor="lt")
                    else:
                        paint.text((left, top), text, font=bold if char.bold else normal, fill=fg, anchor="lt")
                    x = stop
            previous = state
        picture.save(args.frames / f"frame-{number:05d}.png")
        frames.append(args.frames / f"frame-{number:05d}.png")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    # A shared palette prevents static frame chrome from changing between frames.
    gif_palette = picture.quantize(colors=128)
    gif_frames = []
    for path in frames:
        with Image.open(path) as frame:
            gif_frames.append(frame.quantize(palette=gif_palette))
    gif_frames[0].save(args.output.with_suffix(".gif"), save_all=True,
                       append_images=gif_frames[1:], duration=100, loop=0, optimize=True)
    picture.save(args.output.with_suffix(".png"))
    print(json.dumps({"seconds": len(frames) / fps, "fps": fps, "width": width,
                      "height": height, "playback_speed": round(speed, 2)}))
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="action", required=True)
    rec = sub.add_parser("capture")
    rec.add_argument("--cwd", type=Path, required=True)
    rec.add_argument("--output", type=Path, required=True)
    rec.add_argument("command", nargs=argparse.REMAINDER)
    view = sub.add_parser("render")
    view.add_argument("cast", type=Path)
    view.add_argument("--frames", type=Path, required=True)
    view.add_argument("--output", type=Path, required=True, help="output basename for GIF and PNG")
    view.add_argument("--seconds", type=float, default=24)
    view.add_argument("--font", type=Path, default=Path("/System/Library/Fonts/Menlo.ttc"))
    args = parser.parse_args()
    if args.action == "render" and args.seconds <= 5:
        parser.error("duration must exceed five seconds")
    return capture(args) if args.action == "capture" else render(args)


if __name__ == "__main__":
    raise SystemExit(main())
