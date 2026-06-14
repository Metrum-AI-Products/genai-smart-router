Before finishing, validate the implementation with direct `python3` checks.

The Harbor verifier is not mounted in the agent container during the edit phase, so `/tests/test.sh` may not exist until after the agent exits. Do not stop just because `/tests/test.sh` is absent.

Use these expected cases as the local self-check:

```python
from two_bucket import measure

assert measure(3, 5, 1, "one") == (4, "one", 5)
assert measure(3, 5, 1, "two") == (8, "two", 3)
assert measure(7, 11, 2, "one") == (14, "one", 11)
assert measure(7, 11, 2, "two") == (18, "two", 7)
assert measure(1, 3, 3, "two") == (1, "two", 0)
assert measure(2, 3, 3, "one") == (2, "two", 2)
assert measure(6, 15, 9, "one") == (10, "two", 0)

for args in [(2, 4, 5, "one"), (2, 4, 3, "one")]:
    try:
        measure(*args)
    except ValueError as exc:
        assert str(exc)
    else:
        raise AssertionError(f"expected ValueError for {args}")
```
