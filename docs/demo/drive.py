"""Drive a real Claude Code session for docs/demo.gif. Run by record.sh.

Under `asciinema rec`, the session's output is copied to stdout, which
asciinema records. With --trust it records nothing: it starts Claude Code once
to accept the folder trust dialog, then exits.
"""
import os
import re
import sys
import time

import pexpect

ROWS, COLS = 30, 100
WORK = os.environ["DEMO_WORK"]
CLAUDE = (f"command claude --setting-sources local --settings {WORK}/settings.json "
          f"--strict-mcp-config --mcp-config {WORK}/mcp.json")
TITLE = r"\033[1maside\033[0m \342\200\224 a side channel for thoughts while you work with coding agents\n\n"

# Terminal queries Claude Code sends at startup, with the replies a terminal
# gives; without them it ignores keys.
REPLIES = {b"\x1b[c": b"\x1b[?62;22c", b"\x1b[?u": b"\x1b[?0u",
           b"\x1b[>0q": b"\x1bP>|xterm(388)\x1b\\"}

trust_only = "--trust" in sys.argv

with open(f"{WORK}/rc.sh", "w") as f:
    f.write(f"alias claude='{CLAUDE}'\nPS1='$ '\nclear\nprintf '{TITLE}'\n")

env = dict(os.environ)
env["PATH"] = f"{WORK}/bin:" + env["PATH"]
env["TERM"] = "xterm-256color"
env["BASH_SILENCE_DEPRECATION_WARNING"] = "1"

p = pexpect.spawn("/bin/bash", ["--noprofile", "--rcfile", f"{WORK}/rc.sh", "-i"],
                  cwd=f"{WORK}/api", env=env, dimensions=(ROWS, COLS))
recording = not trust_only
seen = b""


def read(timeout):
    """Read what's available, copy it to the recording, answer queries."""
    global seen
    try:
        chunk = p.read_nonblocking(65536, timeout=timeout)
    except pexpect.TIMEOUT:
        return b""
    if recording:
        sys.stdout.buffer.write(chunk)
        sys.stdout.buffer.flush()
    for q, r in REPLIES.items():
        if q in chunk:
            p.send(r)
    seen = (seen + chunk)[-20000:]
    return chunk


def wait_for(pattern, timeout=60):
    """Wait for pattern in output from now on. Returns the index that matched."""
    global seen
    rxs = [re.compile(x) for x in (pattern if isinstance(pattern, list) else [pattern])]
    seen = b""
    end = time.time() + timeout
    while time.time() < end:
        read(0.2)
        for i, rx in enumerate(rxs):
            if rx.search(seen):
                return i
    raise TimeoutError(pattern)


def idle(quiet=2.5, limit=120):
    """Wait until nothing is printed for `quiet` seconds."""
    end = time.time() + limit
    last = time.time()
    while time.time() < end:
        if read(0.2):
            last = time.time()
        elif time.time() - last >= quiet:
            return


def pause(t):
    end = time.time() + t
    while time.time() < end:
        read(0.1)


def typed(text, delay=0.045):
    for ch in text:
        p.send(ch)
        pause(delay)


def enter():
    p.send("\r")


def quit_claude():
    p.send("\x04")
    pause(0.6)
    p.send("\x04")
    wait_for(rb"\$ ", timeout=30)


wait_for(rb"\$ ")

if trust_only:
    p.sendline("claude")
    # Claude Code draws each word separately, so match single words.
    if wait_for([rb"confirm", rb"effort"]) == 0:
        pause(1.5)  # keys sent while the dialog is still drawing are dropped
        p.send("\x1b[B")  # "Yes, I trust this folder"
        pause(1.0)
        enter()
        wait_for(rb"effort")
    idle(quiet=1.5, limit=20)
    quit_claude()
    p.sendline("exit")
    p.expect(pexpect.EOF, timeout=10)
    sys.exit(0)

pause(1.5)
typed("claude")
enter()
wait_for(rb"effort")
idle(quiet=1.5, limit=20)
pause(0.5)

typed("In one sentence, what does this repo do?")
pause(0.4)
enter()
idle(quiet=3.0)
pause(1.0)

typed(">> token cache TTL looks too long, check with infra before shipping")
pause(0.6)
enter()
wait_for(rb"Jotted", timeout=30)
idle(quiet=1.5, limit=10)
pause(2.0)

typed("what's in my jot inbox?")
pause(0.4)
enter()
idle(quiet=3.0)
pause(2.5)

quit_claude()
pause(1.0)

for cmd, wait in [("aside inbox", 2.5),
                  ('aside done 1 --note "raised with infra"', 2.0),
                  ("aside inbox", 2.5)]:
    typed(cmd)
    pause(0.3)
    enter()
    wait_for(rb"\$ ", timeout=15)
    pause(wait)

p.sendline("exit")
p.expect(pexpect.EOF, timeout=10)
