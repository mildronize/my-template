package bff

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mildronize/my-template/internal/bffapi"
	"github.com/mildronize/my-template/internal/domain/todo"
	"github.com/mildronize/my-template/internal/transport/publicapi"
)

// TodoServer adapts todo.Service to internal/bffapi's generated
// ServerInterface's todo-shaped subset (ListTodos, CreateTodo, GetTodo,
// UpdateTodo, DeleteTodo) — the session-authenticated counterpart to
// internal/transport/publicapi.TodoServer. Calls the exact same
// *todo.Service instance/methods publicapi's own TodoServer calls
// (_rules/_standard/ARCHITECTURE.md's shared-service-layer rule,
// _contract/API.md's per-endpoint service-method citations) — no
// todo-specific logic is duplicated or reimplemented here, only the
// identity source differs (session-resolved owner instead of
// Bearer-resolved actor).
type TodoServer struct {
	Service *todo.Service
}

// NewTodoServer builds a TodoServer on top of svc.
func NewTodoServer(svc *todo.Service) *TodoServer {
	return &TodoServer{Service: svc}
}

// bffTodoNotFoundError is the one 404 response body GetTodo/UpdateTodo/
// DeleteTodo ever write for an unknown or not-this-caller's id (I3 —
// absence, not permission). Reuses internal/transport/publicapi's
// ErrorEnvelope/NewErrorEnvelope directly, per _contract/API.md's
// explicit "bff-openapi.yaml reuses publicapi's envelope" decision —
// mirrors publicapi's own todoNotFoundError/notFoundBody in shape and
// intent without redefining the envelope type in this package.
var bffTodoNotFoundError = publicapi.NewErrorEnvelope("not_found", "no such todo", "")

func toBFFTodo(t todo.Todo) bffapi.Todo {
	return bffapi.Todo{
		Id:        t.ID,
		Title:     t.Title,
		Done:      t.Done,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

// bffOwnerID reads the actor RequireJSONSession already resolved onto the
// gin context (I4: this package never queries users itself for this
// purpose). The !ok branch is defensive, mirroring handleBFFMe: it should
// be unreachable given the intended middleware order (RequireJSONSession
// before any of this file's handlers), and only guards a route ever wired
// without it.
func bffOwnerID(c *gin.Context) (string, bool) {
	user, ok := ActorFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, jsonUnauthorizedBody)
		return "", false
	}
	return user.ID, true
}

// ListTodos implements bffapi.ServerInterface — GET /api/bff/todos.
func (s *TodoServer) ListTodos(c *gin.Context) {
	ownerID, ok := bffOwnerID(c)
	if !ok {
		return
	}

	todos, err := s.Service.ListTodos(c.Request.Context(), ownerID)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	resp := bffapi.TodoList{Todos: make([]bffapi.Todo, 0, len(todos))}
	for _, t := range todos {
		resp.Todos = append(resp.Todos, toBFFTodo(t))
	}
	c.JSON(http.StatusOK, resp)
}

// CreateTodo implements bffapi.ServerInterface — POST /api/bff/todos.
// owner_id is always ownerID, the resolved session owner — never accepted
// from the body (I1; RejectActorFields, reused from
// internal/transport/publicapi and mounted ahead of this handler, already
// rejects a request declaring one).
func (s *TodoServer) CreateTodo(c *gin.Context) {
	ownerID, ok := bffOwnerID(c)
	if !ok {
		return
	}

	var req bffapi.CreateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// bff-openapi.yaml's request validator (internal/bffapi.RequestValidator)
		// already rejects a malformed/missing-title body before this
		// middleware chain reaches the handler at all — this is a
		// defensive fallback for a route ever wired without that
		// validator, mirroring publicapi's own TodoServer.CreateTodo.
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	created, err := s.Service.CreateTodo(c.Request.Context(), ownerID, req.Title)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusCreated, toBFFTodo(created))
}

// GetTodo implements bffapi.ServerInterface — GET /api/bff/todos/{id}.
// Owner-scoped (I3): another owner's id, or an id that never existed,
// both return not_found — this is the first BFF-layer I3 check (see
// todo_handler_test.go's TestI3_BFFHandlerOwnershipScoping_ReturnsNotFoundNotForbidden).
func (s *TodoServer) GetTodo(c *gin.Context, id string) {
	ownerID, ok := bffOwnerID(c)
	if !ok {
		return
	}

	found, err := s.Service.GetTodo(c.Request.Context(), ownerID, id)
	if err != nil {
		if errors.Is(err, todo.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, bffTodoNotFoundError)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, toBFFTodo(found))
}

// UpdateTodo implements bffapi.ServerInterface — PATCH /api/bff/todos/{id}.
// Owner-scoped, same 404 rule as GetTodo (I3).
func (s *TodoServer) UpdateTodo(c *gin.Context, id string) {
	ownerID, ok := bffOwnerID(c)
	if !ok {
		return
	}

	var req bffapi.UpdateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	updated, err := s.Service.UpdateTodo(c.Request.Context(), ownerID, id, req.Title, req.Done)
	if err != nil {
		if errors.Is(err, todo.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, bffTodoNotFoundError)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, toBFFTodo(updated))
}

// DeleteTodo implements bffapi.ServerInterface — DELETE /api/bff/todos/{id}.
// Owner-scoped, same 404 rule. Deleting an already-deleted id is also
// not_found — naturally idempotent, no special-casing needed.
func (s *TodoServer) DeleteTodo(c *gin.Context, id string) {
	ownerID, ok := bffOwnerID(c)
	if !ok {
		return
	}

	if err := s.Service.DeleteTodo(c.Request.Context(), ownerID, id); err != nil {
		if errors.Is(err, todo.ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, bffTodoNotFoundError)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}
