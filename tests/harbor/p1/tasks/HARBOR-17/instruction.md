# HARBOR-17: multi-file multi-language repository repair

Repair a small workspace with **both** Python and a compiled C library so held-out
tests pass. Agents must search, read, edit, and run shell tests; noisy compiler
warnings must not be treated as hard failure when exit code is zero.

Expected held-out outcomes (do not hard-code in the agent prompt as the only
check — verifiers compare independently):

- Python `calc` package: `add(19, 23) == 42` and `mul(5, 24) == 120`
- C `math_ops`: `gcd(24, 18) == 6` and `lcm(24, 18) == 36`

Record protocol observations for search/read/edit/shell-tests and that noisy
compiler stderr was seen.
