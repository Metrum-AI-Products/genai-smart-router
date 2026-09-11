# HARBOR-05 · Large streamed write/patch argument

Apply a large patch to `target.txt`. The patch argument is delivered as many streamed JSON fragments containing escaped strings and Unicode.

Reconstruct the exact argument bytes before writing. Truncated, corrupted, or duplicated fragments must fail verification. The final file must match the expected SHA-256.
