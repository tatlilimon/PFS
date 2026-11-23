"""Drive an interactive shell through a pseudo-terminal.

Usage: python3 -I drive.py '<argv json>' '<steps json>' <timeout seconds>

Each step is a list of input lines that are typed together; after a step the
driver types a sync command and waits until its output appears, so the next
step starts only when the shell is idle. The whole terminal transcript is
printed on stdout at the end. Used by wrapper_test.go; stdlib only.
"""

import json
import os
import pty
import select
import sys
import time


def main() -> int:
    argv = json.loads(sys.argv[1])
    steps = json.loads(sys.argv[2])
    timeout = float(sys.argv[3])

    pid, fd = pty.fork()
    if pid == 0:
        os.execvp(argv[0], argv)

    buf = bytearray()

    def read_until(token: bytes, deadline: float) -> None:
        while token not in buf:
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
        for lines in steps:
            for line in lines:
                os.write(fd, line.encode() + b"\r")
            settle()
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
