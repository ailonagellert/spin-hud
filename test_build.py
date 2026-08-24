#!/usr/bin/env python3
"""Build and parser validation for spin-hud."""
from __future__ import annotations

import sys
from spin_hud import _self_check


def main() -> int:
    return _self_check()


if __name__ == "__main__":
    sys.exit(main())
