# Layouts & Components

## Layouts

You can opt-in to layouts using an HTML comment:

```html
<!-- layout: components/layouts/main.html -->
```

Use `{{ define "content" }}` inside `index.html`, and `{{ template "content" . }}` in your layout.

## Components

Create reusable blocks in `components/` using `{{ define "Name" }}`. Call with:

```html
{{ template "Card" . }}
```
