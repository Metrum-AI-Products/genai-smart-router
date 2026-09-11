# Normalize strings for the Harbor repair task.

Rules (also listed in cases.json):
1. Strip leading and trailing ASCII whitespace.
2. Collapse internal whitespace runs to a single space.
3. Lowercase the result.
4. Edge: empty / whitespace-only input must become an empty string (not None).
