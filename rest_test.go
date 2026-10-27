package apitest

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type post struct {
	ID     json.RawMessage `json:"id"`
	Title  string          `json:"title"`
	Body   string          `json:"body"`
	UserID int             `json:"userId"`
}

func TestRESTPostLifecycle(t *testing.T) {
	c := newREST(t)
	initial := post{Title: "Домашняя работа " + c.sandbox, Body: "Текст должен сохраниться после PATCH", UserID: 1}
	createdResponse := c.request(http.MethodPost, "/posts", map[string]any{
		"title": initial.Title, "body": initial.Body, "userId": initial.UserID,
	}, "")
	checkStatus(t, createdResponse, http.StatusCreated)
	created := readJSON[post](t, createdResponse)
	id := resourceID(t, created.ID)
	path := "/posts/" + url.PathEscape(id)
	t.Cleanup(func() {
		r := c.request(http.MethodDelete, path, nil, "")
		checkStatus(t, r, http.StatusOK)
	})
	equal(t, "POST title", created.Title, initial.Title)
	equal(t, "POST body", created.Body, initial.Body)
	equal(t, "POST userId", created.UserID, initial.UserID)

	newTitle := "Изменённый заголовок " + c.sandbox
	patchedResponse := c.request(http.MethodPatch, path, map[string]string{"title": newTitle}, "")
	checkStatus(t, patchedResponse, http.StatusOK)
	patched := readJSON[post](t, patchedResponse)
	equal(t, "PATCH id", resourceID(t, patched.ID), id)
	equal(t, "PATCH title", patched.Title, newTitle)
	equal(t, "PATCH body", patched.Body, initial.Body)
	equal(t, "PATCH userId", patched.UserID, initial.UserID)

	fetchedResponse := c.request(http.MethodGet, path, nil, "")
	checkStatus(t, fetchedResponse, http.StatusOK)
	fetched := readJSON[post](t, fetchedResponse)
	equal(t, "GET id", resourceID(t, fetched.ID), id)
	equal(t, "GET title", fetched.Title, newTitle)
	equal(t, "GET body", fetched.Body, initial.Body)
	equal(t, "GET userId", fetched.UserID, initial.UserID)
}

type user struct {
	ID       json.RawMessage `json:"id"`
	Username string          `json:"username"`
	Name     string          `json:"name"`
	Email    string          `json:"email"`
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	User        user   `json:"user"`
}

func TestRESTLoginAndBearer(t *testing.T) {
	c := newREST(t)
	const username, password = "Bret", "password123"
	email := "hw2-" + c.sandbox + "@example.com"
	const name = "Homework User"

	// Старые данные входа на стенде дают 401. Готовим Bret в своей сессии.
	registeredResponse := c.request(http.MethodPost, "/auth/register", map[string]string{
		"username": username, "password": password, "email": email, "name": name,
	}, "")
	checkStatus(t, registeredResponse, http.StatusCreated)
	registered := readJSON[loginResponse](t, registeredResponse)
	id := resourceID(t, registered.User.ID)

	r := c.request(http.MethodPost, "/auth/login", map[string]string{
		"username": username, "password": password,
	}, "")
	checkStatus(t, r, http.StatusOK)
	login := readJSON[loginResponse](t, r)
	notEmpty(t, "access_token", login.AccessToken)
	equal(t, "token_type", strings.ToLower(login.TokenType), "bearer")
	equal(t, "login user.id", resourceID(t, login.User.ID), id)
	equal(t, "login username", login.User.Username, username)

	// Клиент не хранит cookies. Доступ здесь даёт именно переданный Bearer token.
	meResponse := c.request(http.MethodGet, "/auth/me", nil, login.AccessToken)
	checkStatus(t, meResponse, http.StatusOK)
	me := readJSON[user](t, meResponse)
	equal(t, "me.id", resourceID(t, me.ID), id)
	equal(t, "me.username", me.Username, username)
	equal(t, "me.email", me.Email, email)
	equal(t, "me.name", me.Name, name)
}

func TestRESTMeRejectsInvalidToken(t *testing.T) {
	for _, tc := range []struct {
		name  string
		token string
	}{
		{"Без_токена", ""},
		{"Неверный_токен", "invalid-homework-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newREST(t)
			r := c.request(http.MethodGet, "/auth/me", nil, tc.token)
			checkStatus(t, r, http.StatusUnauthorized)
			body := readJSON[struct {
				Message string          `json:"message"`
				Status  int             `json:"status"`
				ID      json.RawMessage `json:"id"`
			}](t, r)
			notEmpty(t, "message", body.Message)
			equal(t, "status в JSON", body.Status, http.StatusUnauthorized)
			if len(body.ID) != 0 && string(body.ID) != "null" {
				t.Errorf("Ответ об отказе не должен содержать id пользователя: %s", body.ID)
			}
		})
	}
}
