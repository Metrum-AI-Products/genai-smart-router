# HARBOR-04 · Tool error then retry

Call `fetch_record` with the correct id `record-42`. The first attempt with a wrong id returns a controlled tool error (`is_error=true`). Correct the arguments and retry.

Write the successful payload to `record.json`. Attempts must stay bounded (≤ 3). Do not crash or retry endlessly.
