# Extra implementation notes

Implement `/app/sublist.go`.

The implementation should keep the existing `Sublist(l1, l2 []int) Relation` function name and return one of the existing relation constants:

- `RelationEqual`
- `RelationSublist`
- `RelationSuperlist`
- `RelationUnequal`

The `Relation` type and constants are supplied by the test harness in `relations.go`. Do not redefine `Relation`, `RelationEqual`, `RelationSublist`, `RelationSuperlist`, or `RelationUnequal` in `sublist.go`.

Do not create or edit `relations.go`; the final Harbor verifier supplies it.

Do not add third-party dependencies.
