# HARBOR-01 · Read / edit / repair

You are repairing a small Python helper.

1. Read `SPEC.md` and `cases.json` in the workspace.
2. Edit `normalize.py` so every case in `cases.json` passes, including the edge case marked `"edge": true`.
3. Do not leave the starter implementation unchanged. A read-only loop that never edits the file fails this task.

Run local checks with:

```bash
python3 -c "from normalize import normalize; print(normalize('  Hello World  '))"
```

Do not write a success marker; an independent verifier grades the artifact.
