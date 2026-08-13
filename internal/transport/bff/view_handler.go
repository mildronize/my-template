package bff

import (
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mildronize/my-template/internal/domain/todo"
	"github.com/mildronize/my-template/internal/identity"
)

// viewTemplate is the entire "minimal server-rendered view" task-4.md
// calls for — html/template from the standard library (no new templating
// dependency), no JS, no client-side routing. Read-only: it exists so
// มายด์ can visually confirm the identity/domain seam works end to end,
// not to be a usable todo app (task-4.md's "The minimal view itself").
// html/template auto-escapes every {{ . }} substitution, so a todo title
// containing HTML can't inject markup into this page.
const viewTemplateSource = `<!doctype html>
<html>
<head><meta charset="utf-8"><title>my-template — {{.User.Handle}}'s todos</title></head>
<body>
<h1>Signed in as {{.User.Handle}}</h1>
{{if .Todos}}
<ul>
{{range .Todos}}
  <li>{{if .Done}}[x]{{else}}[ ]{{end}} {{.Title}}</li>
{{end}}
</ul>
{{else}}
<p>No todos yet.</p>
{{end}}
</body>
</html>
`

var viewTemplate = template.Must(template.New("bff-view").Parse(viewTemplateSource))

// viewData is what viewTemplate renders.
type viewData struct {
	User  identity.User
	Todos []todo.Todo
}

// NewViewHandler builds GET / (task-4.md "The minimal view itself",
// _contract/API.md). It must run behind RequireSession — it reads the
// actor RequireSession already resolved via ActorFromContext (I4: this
// package never queries users itself) and calls
// internal/domain/todo.Service.ListTodos, the exact same service
// internal/transport/publicapi's own todo handler calls
// (ARCHITECTURE.md's shared-service-layer rule) — no todo-specific logic
// lives in this package.
func NewViewHandler(todoSvc *todo.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := ActorFromContext(c)
		if !ok {
			// Unreachable when mounted behind RequireSession, but fails
			// safe (redirect, not a panic or a 500 leaking internals) if
			// it's ever wired without that middleware ahead of it.
			c.Redirect(http.StatusFound, "/login")
			return
		}

		todos, err := todoSvc.ListTodos(c.Request.Context(), user.ID)
		if err != nil {
			c.String(http.StatusInternalServerError, "failed to load todos")
			return
		}

		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Status(http.StatusOK)
		_ = viewTemplate.Execute(c.Writer, viewData{User: user, Todos: todos})
	}
}
