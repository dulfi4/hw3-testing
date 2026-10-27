package apitest

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type graphQLError struct {
	Message string `json:"message"`
}

type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphQLError  `json:"errors"`
}

func (c *apiClient) graphql(query string, variables map[string]any) response {
	c.t.Helper()
	return c.request(http.MethodPost, "", map[string]any{"query": query, "variables": variables}, "")
}

func graphQLData[T any](t *testing.T, r response) T {
	t.Helper()
	checkStatus(t, r, http.StatusOK)
	envelope := readJSON[graphQLResponse](t, r)
	if len(envelope.Errors) != 0 {
		t.Fatalf("В успешном GraphQL ответе есть errors: %+v", envelope.Errors)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		t.Fatal("В GraphQL ответе отсутствует data")
	}
	var result T
	if err := json.Unmarshal(envelope.Data, &result); err != nil {
		t.Fatalf("Чтение GraphQL data: %v", err)
	}
	return result
}

func TestGraphQLQuery(t *testing.T) {
	for _, name := range []string{"Oleg", "Олег"} {
		t.Run(name, func(t *testing.T) {
			c := newGraphQL(t)
			r := c.graphql(`query Hello($person: PersonInput!) { hello(person: $person) }`,
				map[string]any{"person": map[string]any{"name": name, "age": 20}})
			data := graphQLData[struct {
				Hello string `json:"hello"`
			}](t, r)
			equal(t, "data.hello", data.Hello, "Hello "+name)
		})
	}
}

const createPerson = `mutation CreatePerson($person: PersonInput!) {
	createPerson(person: $person) { id name age }
}`

func TestGraphQLMutation(t *testing.T) {
	c := newGraphQL(t)
	name := "Student " + randomID(t)
	const age = 20
	r := c.graphql(createPerson, map[string]any{"person": map[string]any{"name": name, "age": age}})
	data := graphQLData[struct {
		Person struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Age  *int   `json:"age"`
		} `json:"createPerson"`
	}](t, r)
	notEmpty(t, "data.createPerson.id", data.Person.ID)
	equal(t, "data.createPerson.name", data.Person.Name, name)
	if data.Person.Age == nil {
		t.Fatal("data.createPerson.age отсутствует или равен null")
	}
	equal(t, "data.createPerson.age", *data.Person.Age, age)
}

func TestGraphQLMutationRejectsInvalidVariables(t *testing.T) {
	for _, tc := range []struct {
		name       string
		person     map[string]any
		errorField string
	}{
		{"Возраст_строкой", map[string]any{"name": "Student", "age": "abc"}, "age"},
		{"Дробный_возраст", map[string]any{"name": "Student", "age": 20.5}, "age"},
		{"Имя_не_передано", map[string]any{"age": 20}, "name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newGraphQL(t)
			r := c.graphql(createPerson, map[string]any{"person": tc.person})
			checkStatus(t, r, http.StatusBadRequest)
			envelope := readJSON[graphQLResponse](t, r)
			if len(envelope.Errors) == 0 {
				t.Fatal("GraphQL должен вернуть непустой errors для некорректных variables")
			}
			for _, err := range envelope.Errors {
				notEmpty(t, "errors.message", err.Message)
			}
			if !strings.Contains(strings.ToLower(envelope.Errors[0].Message), tc.errorField) {
				t.Errorf("Ошибка не указывает на поле %s: %s", tc.errorField, envelope.Errors[0].Message)
			}
			if len(envelope.Data) != 0 && string(envelope.Data) != "null" {
				t.Errorf("Невалидная mutation не должна возвращать data: %s", envelope.Data)
			}
		})
	}
}
