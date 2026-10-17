# api

JSON Schemas (draft-07) for the contracts between the body and the harness.
See docs/plans/roadmap.md for what belongs here.

- `state.schema.json`: the state card from `GET /state`. Owned by slice 1.
- `action.schema.json`: request bodies for `POST /action` (and `POST /eval`
  without `type`). The body validates requests against it.

Generated code lives with its consumer and is never hand-edited. The body's
TypeScript types are in `body/src/api/`; regenerate them with
`cd body && npm run gen` after changing a schema.
