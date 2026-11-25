package tasks

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// Service implements adrientv1.TasksServiceServer over a Repository.
type Service struct {
	repo *Repository
}

// accessErr maps an access-check failure to a gRPC status.
func accessErr(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return status.Error(codes.NotFound, "not found")
	case errors.Is(err, ErrForbidden):
		return status.Error(codes.PermissionDenied, "you do not have access to this list")
	}
	return status.Errorf(codes.Internal, "access check: %v", err)
}

// NewService constructs a Service.
func NewService(repo *Repository) *Service { return &Service{repo: repo} }

func callerCtx(ctx context.Context) (orgID, userID string, err error) {
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return "", "", status.Error(codes.Unauthenticated, "no session")
	}
	o, ok := auth.OrgFromContext(ctx)
	if !ok {
		return "", "", status.Error(codes.Internal, "missing org context")
	}
	return o.ID, u.ID, nil
}

func listToProto(l List) *adrientv1.TaskList {
	return &adrientv1.TaskList{
		Id:          l.ID,
		OrgId:       l.OrgID,
		OwnerUserId: l.OwnerUserID,
		Name:        l.Name,
		Position:    l.Position,
		CreatedAt:   l.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func taskToProto(t TaskItem) *adrientv1.Task {
	var dueAt, completedAt string
	if t.DueAt != nil {
		dueAt = t.DueAt.UTC().Format(time.RFC3339)
	}
	if t.CompletedAt != nil {
		completedAt = t.CompletedAt.UTC().Format(time.RFC3339)
	}
	return &adrientv1.Task{
		Id:           t.ID,
		OrgId:        t.OrgID,
		ListId:       t.ListID,
		OwnerUserId:  t.OwnerUserID,
		Title:        t.Title,
		Notes:        t.Notes,
		DueAt:        dueAt,
		Completed:    t.Completed,
		CompletedAt:  completedAt,
		ParentTaskId: t.ParentTaskID,
		Position:     t.Position,
		CreatedAt:    t.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:    t.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// parseOptionalTime parses an RFC-3339 string (or empty) into *time.Time.
func parseOptionalTime(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid time %q: %v", s, err)
	}
	return &t, nil
}

// ----- Task list RPCs -----

func (s *Service) ListLists(ctx context.Context, _ *adrientv1.ListTaskListsRequest) (*adrientv1.ListTaskListsResponse, error) {
	orgID, userID, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	lists, err := s.repo.ListLists(ctx, orgID, userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list task lists: %v", err)
	}
	resp := &adrientv1.ListTaskListsResponse{Lists: make([]*adrientv1.TaskList, 0, len(lists))}
	for _, l := range lists {
		resp.Lists = append(resp.Lists, listToProto(l))
	}
	return resp, nil
}

func (s *Service) CreateList(ctx context.Context, req *adrientv1.CreateTaskListRequest) (*adrientv1.TaskList, error) {
	orgID, userID, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	l, err := s.repo.CreateList(ctx, orgID, userID, ListFields{Name: req.GetName()})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create task list: %v", err)
	}
	return listToProto(l), nil
}

func (s *Service) UpdateList(ctx context.Context, req *adrientv1.UpdateTaskListRequest) (*adrientv1.TaskList, error) {
	orgID, userID, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	if role, rerr := s.repo.Role(ctx, orgID, userID, req.GetId()); rerr != nil {
		return nil, accessErr(rerr)
	} else if role != RoleOwner {
		return nil, accessErr(ErrForbidden)
	}
	l, err := s.repo.UpdateList(ctx, orgID, req.GetId(), ListFields{Name: req.GetName()})
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "task list not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update task list: %v", err)
	}
	return listToProto(l), nil
}

func (s *Service) DeleteList(ctx context.Context, req *adrientv1.DeleteTaskListRequest) (*adrientv1.DeleteTaskListResponse, error) {
	orgID, userID, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	if role, rerr := s.repo.Role(ctx, orgID, userID, req.GetId()); rerr != nil {
		return nil, accessErr(rerr)
	} else if role != RoleOwner {
		return nil, accessErr(ErrForbidden)
	}
	err = s.repo.DeleteList(ctx, orgID, req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "task list not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "delete task list: %v", err)
	}
	return &adrientv1.DeleteTaskListResponse{}, nil
}

// ----- Task RPCs -----

func (s *Service) ListTasks(ctx context.Context, req *adrientv1.ListTasksRequest) (*adrientv1.ListTasksResponse, error) {
	orgID, userID, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Require(ctx, orgID, userID, req.GetListId(), false); err != nil {
		return nil, accessErr(err)
	}
	tasks, err := s.repo.ListTasks(ctx, orgID, req.GetListId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list tasks: %v", err)
	}
	resp := &adrientv1.ListTasksResponse{Tasks: make([]*adrientv1.Task, 0, len(tasks))}
	for _, t := range tasks {
		resp.Tasks = append(resp.Tasks, taskToProto(t))
	}
	return resp, nil
}

func (s *Service) CreateTask(ctx context.Context, req *adrientv1.CreateTaskRequest) (*adrientv1.Task, error) {
	orgID, userID, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	dueAt, err := parseOptionalTime(req.GetDueAt())
	if err != nil {
		return nil, err
	}
	if err := s.repo.Require(ctx, orgID, userID, req.GetListId(), true); err != nil {
		return nil, accessErr(err)
	}
	t, err := s.repo.CreateTask(ctx, orgID, req.GetListId(), userID, TaskFields{
		Title:        req.GetTitle(),
		Notes:        req.GetNotes(),
		DueAt:        dueAt,
		ParentTaskID: req.GetParentTaskId(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create task: %v", err)
	}
	return taskToProto(t), nil
}

func (s *Service) UpdateTask(ctx context.Context, req *adrientv1.UpdateTaskRequest) (*adrientv1.Task, error) {
	orgID, userID, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.RequireTask(ctx, orgID, userID, req.GetId(), true); err != nil {
		return nil, accessErr(err)
	}
	dueAt, err := parseOptionalTime(req.GetDueAt())
	if err != nil {
		return nil, err
	}
	t, err := s.repo.UpdateTask(ctx, orgID, req.GetId(), TaskFields{
		Title:        req.GetTitle(),
		Notes:        req.GetNotes(),
		DueAt:        dueAt,
		ParentTaskID: req.GetParentTaskId(),
	})
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "task not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update task: %v", err)
	}
	return taskToProto(t), nil
}

func (s *Service) DeleteTask(ctx context.Context, req *adrientv1.DeleteTaskRequest) (*adrientv1.DeleteTaskResponse, error) {
	orgID, userID, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.RequireTask(ctx, orgID, userID, req.GetId(), true); err != nil {
		return nil, accessErr(err)
	}
	err = s.repo.DeleteTask(ctx, orgID, req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "task not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "delete task: %v", err)
	}
	return &adrientv1.DeleteTaskResponse{}, nil
}

func (s *Service) ToggleTask(ctx context.Context, req *adrientv1.ToggleTaskRequest) (*adrientv1.Task, error) {
	orgID, userID, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.RequireTask(ctx, orgID, userID, req.GetId(), true); err != nil {
		return nil, accessErr(err)
	}
	t, err := s.repo.ToggleTask(ctx, orgID, req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "task not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "toggle task: %v", err)
	}
	// A repeating task schedules its next occurrence when completed.
	_, _ = s.repo.SpawnNext(ctx, t)
	return taskToProto(t), nil
}

func (s *Service) ReorderTask(ctx context.Context, req *adrientv1.ReorderTaskRequest) (*adrientv1.Task, error) {
	orgID, userID, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.RequireTask(ctx, orgID, userID, req.GetId(), true); err != nil {
		return nil, accessErr(err)
	}
	t, err := s.repo.ReorderTask(ctx, orgID, req.GetId(), req.GetPosition())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "task not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "reorder task: %v", err)
	}
	return taskToProto(t), nil
}
