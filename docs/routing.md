# Routing

Barry supports file-based routing using the `routes/` directory.

## Static Routes

```
routes/about/index.html   => /about
routes/contact/index.html => /contact
```

## Dynamic Routes

Use square brackets in folder names:

```
routes/blog/[slug]/index.html => /blog/my-article
```

Params like `slug` are passed into your `index.server.go`.
