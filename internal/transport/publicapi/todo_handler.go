// Package publicapi is the REST transport surface for agents/skills,
// key-authenticated (_contract/API.md) — the public API distinct from the
// owner-facing internal/transport/bff surface a later task adds. It holds
// every HTTP-facing piece for both the todo domain and identity: the
// generated-interface adapters (todo_handler.go, me_handler.go,
// keys_handler.go) and the actor-resolution middleware (middleware.go,
// moved here from internal/identity's old handler.go/middleware_handler.go
// — ARCHITECTURE.md: "Why transport is not inside a domain module
// anymore"). No domain module or internal/identity may import this
// package back (ARCHITECTURE.md rule 4) — dependencies point one way,
// from here down into internal/domain/* and internal/identity, never the
// reverse.
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

// notFoundError, unauthorizedError are the two error bodies this handler
// ever writes on its own (validation_error responses come from
// internal/api's openapi request validator instead, before a request even
// reaches here — API.md).
var notFoundError = newAPIError("not_found", "no such todo")

var unauthorizedError = newAPIError("unauthorized", "authentication required")

func newAPIError(code, message string) api.Error {
	e := api.Error{}
	e.Error.Code = code
	e.Error.Message = message
	return e
}

// actorID reads the actor RequireActor (middleware.go, this package)
// already resolved onto the gin context (ActorFromContext) — this handler
// never queries users/api_keys itself, nor does it ever look a todo up by
// id alone without also knowing whose it must be (I4). The !ok branch is
// defensive, mirroring handleMe: it should be unreachable given the
// intended middleware order (RejectActorFields, RequireActor, then this
// handler), and is here only in case a route is ever wired without that
// chain.
func actorID(c *gin.Context) (string, bool) {
	user, ok := ActorFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedError)
		return "", false
	}
	return user.ID, true
}

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
			c.AbortWithStatusJSON(http.StatusNotFound, notFoundError)
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
			c.AbortWithStatusJSON(http.StatusNotFound, notFoundError)
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
			c.AbortWithStatusJSON(http.StatusNotFound, notFoundError)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}
