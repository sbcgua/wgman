
- Primary solution design and user behavior defined by [docs/SPEC.md](./docs/SPEC.md). Update that spec when improvement changes command behavior or config format.
- [README.md](./README.md) is the user-facing documentation; Keep it updated with the new features and usage details. Do not link development logs from it.
- Durable implementation conventions belong in [docs/NOTES.md](./docs/NOTES.md). Do not duplicate development history in it. Update `docs/NOTES.md` only with general conventions or non-obvious facts that would help a future agent safely change the project.
- The project will be built and used under Linux. If agent runs in Windows:
  - limit windows specific comments to any documentation (especially avoid paths with real username in it, use env variables e.g. `$USERPROFILE` instead)
  - highlight to the user, if there are things to double-check under linux env, what might not work there for some reason.
