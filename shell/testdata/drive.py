"""Drive an interactive shell through a pseudo-terminal.

Usage: python3 -I drive.py '<argv json>' '<steps json>' <timeout seconds>

Each step is a list of items typed in order. A string item is a line (sent
with Enter). An object item {"expect": text, "keys": keys} waits until text
appears in the output produced after the previous expectation, then sends
keys without Enter (for driving the pfs TUI). After a step the driver types a
sync command and waits until its output appears, so the next step starts only
when the shell is idle. The whole terminal transcript is
printed on stdout at the end. Used by wrapper_test.go; stdlib only.
"""

import fcntl
import json
import os
import pty
import select
import struct
import sys
import termios
import time


def main() -> int:
    argv = json.loads(sys.argv[1])
    steps = json.loads(sys.argv[2])
    timeout = float(sys.argv[3])

    pid, fd = pty.fork()
    if pid == 0:
        os.execvp(argv[0], argv)
    # A real terminal has a size; without one, full-screen programs draw nothing.
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))

    buf = bytearray()
    pos = 0  # expectations only match output after this offset

    def read_until(token: bytes, deadline: float, start: int = 0) -> int:
        while buf.find(token, start) < 0:
            remaining = deadline - time.time()
            if remaining <= 0:
                raise TimeoutError("timed out waiting for %r" % token)
            ready, _, _ = select.select([fd], [], [], remaining)
            if fd in ready:
                try:
                    data = os.read(fd, 65536)
                except OSError:
                    data = b""
                if not data:
                    raise EOFError("shell exited while waiting for %r" % token)
                buf.extend(data)
        return buf.find(token, start) + len(token)

    status = 0
    try:
        sync = 0

        def settle() -> None:
            nonlocal sync
            sync += 1
            # The typed text contains "%s", the output contains the number:
            # only the output matches the token we wait for.
            os.write(fd, ("printf 'SYNC_%%s_DONE\\n' %d\r" % sync).encode())
            read_until(("SYNC_%d_DONE" % sync).encode(), time.time() + timeout)

        settle()  # the shell has started and sourced its rc file
        for items in steps:
            drove_tui = False
            for item in items:
                if isinstance(item, dict):
                    pos = read_until(item["expect"].encode(), time.time() + timeout, pos)
                    os.write(fd, item.get("keys", "").encode())
                    drove_tui = True
                else:
                    os.write(fd, item.encode() + b"\r")
            if drove_tui:
                # Typing ahead would go to the TUI, not the shell: wait until
                # the program has exited and the prompt is back.
                read_until(b"PFSPROMPT> ", time.time() + timeout, pos)
            settle()
            pos = len(buf)
        os.write(fd, b"exit\r")
        deadline = time.time() + timeout
        while time.time() < deadline:
            done, _ = os.waitpid(pid, os.WNOHANG)
            if done:
                break
            ready, _, _ = select.select([fd], [], [], 0.1)
            if fd in ready:
                try:
                    data = os.read(fd, 65536)
                except OSError:
                    data = b""
                if data:
                    buf.extend(data)
    except Exception as exc:  # report and keep the transcript
        sys.stderr.write("driver error: %s\n" % exc)
        status = 1
        try:
            os.kill(pid, 9)
        except OSError:
            pass
    sys.stdout.write(buf.decode("utf-8", "replace"))
    return status


if __name__ == "__main__":
    sys.exit(main())
