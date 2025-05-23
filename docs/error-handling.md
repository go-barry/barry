# Error Handling

### Catch-all

`routes/_error/index.html`

### Per Status Code

Add files like:

- `routes/_error/404.html`
- `routes/_error/500.html`

Template receives:

```go
.StatusCode
.Message
.Path
```
