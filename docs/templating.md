# Templating

Barry uses Go’s built-in `html/template` engine.

- Data is passed via `map[string]interface{}`
- Use `{{ .Title }}`, `{{ range .Items }}`, etc.

Example:

```html
<h1>{{ .Title }}</h1>
<ul>
  {{ range .Items }}
    <li>{{ . }}</li>
  {{ end }}
</ul>
```
