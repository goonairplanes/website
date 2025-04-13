package users

import (
	"goonairplanes/core"
	"net/http"
	"strconv"
	"sync"
)

type User struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Username string `json:"username,omitempty"`
}

func init() {
	core.RegisterAPIHandler("/api/users", http.MethodGet, GetUsers)
	core.RegisterAPIHandler("/api/users", http.MethodPost, CreateUser)
	core.RegisterAPIHandler("/api/users/[id]", http.MethodGet, GetUserByID)
	core.RegisterAPIHandler("/api/users/[id]", http.MethodPut, UpdateUserByID)
	core.RegisterAPIHandler("/api/users/[id]", http.MethodDelete, DeleteUserByID)
}

var mockUsers = []User{
	{ID: 1, Name: "John Doe", Email: "john@example.com", Username: "johndoe"},
	{ID: 2, Name: "Jane Smith", Email: "jane@example.com", Username: "janesmith"},
}
var nextUserID int64 = 3
var userMutex sync.Mutex

func GetUsers(ctx *core.APIContext) {
	page, perPage := core.GetPaginationParams(ctx.Request, 10)

	userMutex.Lock()
	totalItems := len(mockUsers)
	startIndex := (page - 1) * perPage
	endIndex := startIndex + perPage
	if startIndex >= totalItems {
		startIndex = totalItems
	}
	if endIndex > totalItems {
		endIndex = totalItems
	}
	pagedUsers := make([]User, 0)
	if startIndex < endIndex {
		pagedUsers = append(pagedUsers, mockUsers[startIndex:endIndex]...)
	}
	userMutex.Unlock()

	meta := core.NewPaginationMeta(page, perPage, totalItems)
	core.RenderPaginated(ctx.Writer, pagedUsers, meta, http.StatusOK)
}

func CreateUser(ctx *core.APIContext) {
	var newUser User
	if err := ctx.ParseBody(&newUser); err != nil {
		ctx.Error("Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if newUser.Name == "" || newUser.Email == "" {
		ctx.Error("Name and email are required", http.StatusBadRequest)
		return
	}

	userMutex.Lock()
	newUser.ID = nextUserID
	nextUserID++
	mockUsers = append(mockUsers, newUser)
	userMutex.Unlock()

	ctx.Success(newUser, http.StatusCreated)
}

func GetUserByID(ctx *core.APIContext) {
	idStr := ctx.Params["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		ctx.Error("Invalid user ID format", http.StatusBadRequest)
		return
	}

	userMutex.Lock()
	var foundUser *User
	for i := range mockUsers {
		if mockUsers[i].ID == id {
			foundUser = &mockUsers[i]
			break
		}
	}
	userMutex.Unlock()

	if foundUser == nil {
		ctx.Error("User not found", http.StatusNotFound)
		return
	}

	ctx.Success(*foundUser, http.StatusOK)
}

func UpdateUserByID(ctx *core.APIContext) {
	idStr := ctx.Params["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		ctx.Error("Invalid user ID format", http.StatusBadRequest)
		return
	}

	var updatedData User
	if err := ctx.ParseBody(&updatedData); err != nil {
		ctx.Error("Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	userMutex.Lock()
	found := false
	for i := range mockUsers {
		if mockUsers[i].ID == id {
			if updatedData.Name != "" {
				mockUsers[i].Name = updatedData.Name
			}
			if updatedData.Email != "" {
				mockUsers[i].Email = updatedData.Email
			}
			updatedData = mockUsers[i]
			found = true
			break
		}
	}
	userMutex.Unlock()

	if !found {
		ctx.Error("User not found", http.StatusNotFound)
		return
	}

	ctx.Success(updatedData, http.StatusOK)
}

func DeleteUserByID(ctx *core.APIContext) {
	idStr := ctx.Params["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		ctx.Error("Invalid user ID format", http.StatusBadRequest)
		return
	}

	userMutex.Lock()
	foundIndex := -1
	for i := range mockUsers {
		if mockUsers[i].ID == id {
			foundIndex = i
			break
		}
	}
	if foundIndex != -1 {
		mockUsers = append(mockUsers[:foundIndex], mockUsers[foundIndex+1:]...)
	}
	userMutex.Unlock()

	if foundIndex == -1 {
		ctx.Error("User not found", http.StatusNotFound)
		return
	}

	ctx.Success(nil, http.StatusNoContent)
}
