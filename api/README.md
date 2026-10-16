# api

JSON Schemas (draft-07) for the contracts between the body and the harness.
See docs/plans/roadmap.md for what belongs here.

- `state.schema.json`: the state card from `GET /state`. Owned by slice 1.
- `action.schema.json`: request bodies for `POST /action` (and `POST /eval`
  without `type`). The body validates requests against it.
- `goal.schema.json`: a typed goal with a machine-checkable `until`
  predicate (`items`, `arrived`, `built`, `delivered`; `judged` reserved).
  Owned by slice 2.

Generated code lives with its consumer and is never hand-edited. The body's
TypeScript types are in `body/src/api/`; regenerate them with
`cd body && npm run gen` after changing a schema. The harness's goal types
are in `harness/goal/goal.gen.go`; regenerate them with
`cd harness && go generate ./goal`.
