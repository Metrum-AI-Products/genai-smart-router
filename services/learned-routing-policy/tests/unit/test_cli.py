# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

import pytest
from lrp.cli import parser


@pytest.mark.parametrize("args", [["serve", "--secret", "PRIVATE-SENTINEL"],
                                  ["featurize", "--seed", "PRIVATE-SENTINEL"]])
def test_parser_never_echoes_invalid_values(args, capsys):
    with pytest.raises(SystemExit):
        parser().parse_args(args)
    assert "PRIVATE-SENTINEL" not in capsys.readouterr().err


def test_cli_seed_and_serial_uncapped_options():
    args = parser().parse_args(["featurize", "--requests", "/data/in", "--out", "/data/out",
                                "--synthetic", "--seed", "7"])
    assert args.seed == 7
    args = parser().parse_args(["fanout", "--requests", "/data/in", "--out", "/data/out",
                                "--targets", "/data/targets", "--allow-uncapped",
                                "--concurrency", "1", "--max-total-cost-usd", "1"])
    assert args.concurrency == 1
