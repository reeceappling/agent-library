---
paths:
  - "src/api/**/*.ts"
  - "src/handlers/**/*.ts"
---

# API Development Standards

## Architecture
* Always use RESTful naming conventions.
* Keep controllers thin; isolate business logic inside dedicated service files.

## Error Handling
* Never return raw database error messages to the client.
* Catch all exceptions and return a standardized JSON error payload: `{ "error": "Message", "code": 400 }`.

## Performance
* Database queries inside loops are strictly forbidden. Use batch operations or `.populate()`.
