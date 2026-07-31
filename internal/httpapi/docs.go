package httpapi

import (
	_ "embed"
	"net/http"
)

// The spec is embedded rather than read from disk so a single binary carries
// its own documentation — the Vercel and container builds ship no source tree.
//
//go:embed openapi.yaml
var openAPISpec []byte

// openAPI serves the machine-readable specification. Point any OpenAPI tool
// (Postman, Insomnia, a client generator) at this URL.
func openAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Write(openAPISpec)
}

// swaggerUI serves the interactive documentation page.
//
// The Swagger UI assets come from a CDN rather than being vendored into the
// repository: they are ~1 MB of JavaScript that would otherwise need to be
// committed and kept up to date. The trade-off is that this page needs
// internet access in the browser to render — the spec at /openapi.yaml is
// self-contained and always available offline.
func swaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(swaggerHTML))
}

const swaggerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Ticketmaster API — reference</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>
    body { margin: 0; background: #fafafa; }
    .topbar { display: none; }
  </style>
</head>
<body>
  <div id="swagger"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function () {
      SwaggerUIBundle({
        url: "/openapi.yaml",
        dom_id: "#swagger",
        deepLinking: true,
        persistAuthorization: true,
        docExpansion: "none",
        defaultModelsExpandDepth: 0,
        tryItOutEnabled: true,
      });
    };
  </script>
</body>
</html>
`
