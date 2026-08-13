package publicapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mildronize/my-template/internal/api"
	"github.com/mildronize/my-template/internal/domain/todo"
)

// TodoServer adapts todo.Service to internal/api's generated
// ServerInterface's todo-shaped subset (ListTodos, CreateTodo, GetTodo,
// UpdateTodo, DeleteTodo). MeServer/KeysServer (me_handler.go/
// keys_handler.go, this package) contribute the identity-shaped subset —
// cmd/server composes all three into one api.ServerInterface
// implementation by embedding, so GET /api/v1/me and /api/v1/keys run
// through the exact same generated-interface, openapi-validated path as
// every other endpoint (task-3, unchanged by this package's move).
type TodoServer struct {
	Service *todo.Service
}

// NewTodoServer builds a TodoServer on top of svc.
func NewTodoServer(svc *todo.Service) *TodoServer {
	return &TodoServer{Service: svc}
}

// todoNotFoundError is the one error body GetTodo/UpdateTodo/DeleteTodo
// ever write for an unknown or not-this-caller's id (validation_error
// responses come from internal/api's openapi request validator instead,
// before a request even reaches here — API.md). Named with a todo- prefix,
// not the bare notFoundError this file used before task-9, so that a fork
// copying this file to <new>_handler.go and renaming Todo -> <New>
// throughout ends up with its own distinctly-named value instead of
// redeclaring this one — mirrors keys_handler.go's own notFoundBody, one
// per handler file, never shared. The unauthorized case has no equivalent
// per-file value: its message never varies by domain, so actorID
// (middleware.go) writes the package's one shared unauthorizedBody
// instead of a second, todo-specific copy of the same text.
var todoNotFoundError = newAPIError("not_found", "no such todo")

func toAPITodo(t todo.Todo) api.Todo {
	return api.Todo{
		Id:        t.ID,
		Title:     t.Title,
		Done:      t.Done,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

// ListTodos implements api.ServerInterface — GET /api/v1/todos.
func (s *TodoServer) ListTodos(c *gin.Context) {
	ownerID, ok := actorID(c)
	if !ok {
		return
	}

	todos, err := s.Service.ListTodos(c.Request.Context(), ownerID)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	resp := api.TodoList{Todos: make([]api.Todo, 0, len(todos))}
	for _, t := range todos {
		resp.Todos = append(resp.Todos, toAPITodo(t))
	}
	c.JSON(http.StatusOK, resp)
}

// CreateTodo implements api.ServerInterface — POST /api/v1/todos.
// owner_id is always ownerID, the resolved actor — never accepted from
// the body (I1; the body's own shape can't even carry an owner/actor
// field, since RejectActorFields already rejected that upstream).
func (s *TodoServer) CreateTodo(c *gin.Context) {
	ownerID, ok := actorID(c)
	if !ok {
		return
	}

	var req api.CreateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// openapi.yaml's request validator (internal/api.RequestValidator)
		// already rejects a malformed/missing-title body before this
		// middleware chain reaches the handler at all (API.md, Done-when
		// 7) — this is a defensive fallback for a route ever wired
		// without that validator, not the primary validation path.
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	created, err := s.Service.CreateTodo(c.Request.Context(), ownerID, req.Title)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusCreated, toAPITodo(created))
}

// GetTodo implements api.ServerInterface — GET /api/v1/todos/{id}.
// Owner-scoped (I3): another owner's id, or an id that never existed,
// both return not_found.
func (s *TodoServer) GetTodo(c *gin.Context, id string) {
	ownerID, ok := actorID(c)
	if !ok {
		return
	}

	found, err := s.Service.GetTodo(c.Request.Context(), ownerID, id)
	if err != nil {
		if errors.Is(err, todo.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, todoNotFoundError)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, toAPITodo(found))
}

// UpdateTodo implements api.ServerInterface — PATCH /api/v1/todos/{id}.
// Owner-scoped, same 404 rule as GetTodo (I3).
func (s *TodoServer) UpdateTodo(c *gin.Context, id string) {
	ownerID, ok := actorID(c)
	if !ok {
		return
	}

	var req api.UpdateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	updated, err := s.Service.UpdateTodo(c.Request.Context(), ownerID, id, req.Title, req.Done)
	if err != nil {
		if errors.Is(err, todo.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, todoNotFoundError)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, toAPITodo(updated))
}

// DeleteTodo implements api.ServerInterface — DELETE /api/v1/todos/{id}.
// Owner-scoped, same 404 rule. Deleting an already-deleted id is also
// not_found — naturally idempotent, no special-casing needed (API.md).
func (s *TodoServer) DeleteTodo(c *gin.Context, id string) {
	ownerID, ok := actorID(c)
	if !ok {
		return
	}

	if err := s.Service.DeleteTodo(c.Request.Context(), ownerID, id); err != nil {
		if errors.Is(err, todo.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, todoNotFoundError)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}
