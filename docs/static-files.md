# Static Files

Place static assets in `public/`.

- `/public/logo.png`  => `/static/logo.png`
- `/public/style.css` => `/static/style.css`

Mount is handled in `server.go` using `http.FileServer`.
