package todo

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mildronize/my-template/internal/api"
	"github.com/mildronize/my-template/internal/identity"
)

// Server adapts Service to internal/api's generated ServerInterface's
// todo-shaped subset (ListTodos, CreateTodo, GetTodo, UpdateTodo,
// DeleteTodo). GetMe lives in internal/identity's own adapter
// (identity.MeServer) — cmd/server composes both into one
// api.ServerInterface implementation by embedding, so GET /api/v1/me runs
// through the exact same generated-interface, openapi-validated path as
// every other endpoint instead of a bespoke route (task-3).
type Server struct {
	Service *Service
}

// NewServer builds a Server on top of svc.
func NewServer(svc *Service) *Server {
	return &Server{Service: svc}
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

// actorID reads the actor RequireActor already resolved onto the gin
// context (identity.ActorFromContext) — this handler never queries
// users/api_keys itself, nor does it ever look a todo up by id alone
// without also knowing whose it must be (I4). The !ok branch is
// defensive, mirroring identity.handleMe: it should be unreachable given
// the intended middleware order (RejectActorFields, RequireActor, then
// this handler), and is here only in case a route is ever wired without
// that chain.
func actorID(c *gin.Context) (string, bool) {
	user, ok := identity.ActorFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, unauthorizedError)
		return "", false
	}
	return user.ID, true
}

func toAPITodo(t Todo) api.Todo {
	return api.Todo{
		Id:        t.ID,
		Title:     t.Title,
		Done:      t.Done,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

// ListTodos implements api.ServerInterface — GET /api/v1/todos.
func (s *Server) ListTodos(c *gin.Context) {
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
// field, since identity.RejectActorFields already rejected that
// upstream).
func (s *Server) CreateTodo(c *gin.Context) {
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
func (s *Server) GetTodo(c *gin.Context, id string) {
	ownerID, ok := actorID(c)
	if !ok {
		return
	}

	found, err := s.Service.GetTodo(c.Request.Context(), ownerID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
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
func (s *Server) UpdateTodo(c *gin.Context, id string) {
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
		if errors.Is(err, ErrNotFound) {
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
func (s *Server) DeleteTodo(c *gin.Context, id string) {
	ownerID, ok := actorID(c)
	if !ok {
		return
	}

	if err := s.Service.DeleteTodo(c.Request.Context(), ownerID, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, notFoundError)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}
