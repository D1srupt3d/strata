#!/usr/bin/env python3
"""Convert a number to its hexadecimal representation.

Usage:
    python3 hex.py 240        -> F0
    python3 hex.py            -> prompts you to type numbers (Ctrl-D to quit)

Accepts plain integers (255), negatives (-16), and numbers already in
other bases if you prefix them (0b1010 for binary, 0o17 for octal).
"""

import sys


def to_hex(text: str) -> str:
    """Parse a number from text and return two-digit hex, e.g. '240' -> 'F0'."""
    # int(text, 0) means "figure out the base from the prefix":
    # plain digits are base 10, 0b... is binary, 0o... is octal, 0x... is hex.
    number = int(text, 0)
    # :02X = uppercase hex, padded with a leading zero to at least 2 digits.
    # Numbers over 255 still print in full (e.g. 4096 -> '1000').
    return f"{number:02X}"


def main() -> None:
    args = sys.argv[1:]

    if args:
        # Numbers were given on the command line: convert each one.
        for arg in args:
            try:
                print(f"{arg} = {to_hex(arg)}")
            except ValueError:
                print(f"error: {arg!r} is not a number I can parse", file=sys.stderr)
                sys.exit(1)
    else:
        # No arguments: interactive mode. Keep asking until Ctrl-D or empty input.
        print("Enter a number to convert to hex (blank line or Ctrl-D to quit):")
        while True:
            try:
                line = input("> ").strip()
            except EOFError:
                break
            if not line:
                break
            try:
                print(to_hex(line))
            except ValueError:
                print(f"error: {line!r} is not a number I can parse")


if __name__ == "__main__":
    main()
