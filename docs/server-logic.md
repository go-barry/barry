# Server Logic

Each route can include an `index.server.go` file which defines a `HandleRequest` function:

```go
func HandleRequest(r *http.Request, params map[string]string) (map[string]interface{}, error)
```

This returns data used to populate your templates.
