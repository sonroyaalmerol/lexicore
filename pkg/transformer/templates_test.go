package transformer

import (
	"context"
	"encoding/base64"
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateTransformer_BasicTemplate(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"fullEmail": "{{.Username}}@example.com",
			"maildir":   "/var/mail/{{.Username}}",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username:   "john",
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(t, "john@example.com", transformed["john"].Attributes["fullEmail"])
	assert.Equal(t, "/var/mail/john", transformed["john"].Attributes["maildir"])
}

func TestTemplateTransformer_MultipleFields(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"displayName": "{{.Username}} ({{.Email}})",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username:   "john",
			Email:      "john@example.com",
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(
		t,
		"john (john@example.com)",
		transformed["john"].Attributes["displayName"],
	)
}

func TestTemplateTransformer_AttributeAccess(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"summary": "User {{.Username}} from {{.Attributes.department}}",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "john",
			Attributes: map[string]any{
				"department": "Engineering",
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(
		t,
		"User john from Engineering",
		transformed["john"].Attributes["summary"],
	)
}

func TestTemplateTransformer_EmptyConfig(t *testing.T) {
	config := map[string]any{}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "john",
			Attributes: map[string]any{
				"existing": "value",
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(t, "value", transformed["john"].Attributes["existing"])
}

func TestTemplateTransformer_InvalidTemplate(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"invalid": "{{.Username",
		},
	}

	_, err := NewTemplateTransformer(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse template")
}

func TestTemplateTransformer_NonStringTemplate(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"number": 123,
			"valid":  "{{.Username}}",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	// Should skip non-string templates
	assert.Len(t, tt.templates, 1)
	assert.NotNil(t, tt.templates["valid"])
	assert.Nil(t, tt.templates["number"])
}

func TestTemplateTransformer_NilAttributes(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"email": "{{.Username}}@example.com",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username:   "john",
			Attributes: nil,
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	require.NotNil(t, transformed["john"].Attributes)
	assert.Equal(t, "john@example.com", transformed["john"].Attributes["email"])
}

func TestTemplateTransformer_MultipleIdentities(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"email": "{{.Username}}@example.com",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {Username: "john", Attributes: make(map[string]any)},
		"jane": {Username: "jane", Attributes: make(map[string]any)},
		"bob":  {Username: "bob", Attributes: make(map[string]any)},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Len(t, transformed, 3)
	assert.Equal(t, "john@example.com", transformed["john"].Attributes["email"])
	assert.Equal(t, "jane@example.com", transformed["jane"].Attributes["email"])
	assert.Equal(t, "bob@example.com", transformed["bob"].Attributes["email"])
}

func TestTemplateTransformer_ComplexTemplate(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"path": "/home/{{.Username}}/{{.Attributes.project}}/data",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "john",
			Attributes: map[string]any{
				"project": "myapp",
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(t, "/home/john/myapp/data", transformed["john"].Attributes["path"])
}

func TestTemplateTransformer_OverwriteExisting(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"email": "{{.Username}}@newdomain.com",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "john",
			Attributes: map[string]any{
				"email": "john@olddomain.com",
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(
		t,
		"john@newdomain.com",
		transformed["john"].Attributes["email"],
	)
}

func TestTemplateTransformer_ArrayExpansion(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"groups":      "{{.Attributes.groupList}}",
			"permissions": "{{.Attributes.perms}}",
			"email":       "{{.Username}}@example.com",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "john",
			Attributes: map[string]any{
				"groupList": []string{"admin", "users", "developers"},
				"perms":     []string{"read", "write", "execute"},
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)

	groups, ok := transformed["john"].Attributes["groups"].([]string)
	require.True(t, ok, "groups should be []string, got %T", transformed["john"].Attributes["groups"])
	assert.Equal(t, []string{"admin", "users", "developers"}, groups)

	perms, ok := transformed["john"].Attributes["permissions"].([]string)
	require.True(t, ok, "permissions should be []string, got %T", transformed["john"].Attributes["permissions"])
	assert.Equal(t, []string{"read", "write", "execute"}, perms)

	assert.Equal(t, "john@example.com", transformed["john"].Attributes["email"])
}

func TestTemplateTransformer_ArrayExpansion_IntSlice(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"ids": "{{.Attributes.userIds}}",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "john",
			Attributes: map[string]any{
				"userIds": []int{100, 200, 300},
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)

	ids, ok := transformed["john"].Attributes["ids"].([]int)
	require.True(t, ok, "ids should be []int, got %T", transformed["john"].Attributes["ids"])
	assert.Equal(t, []int{100, 200, 300}, ids)
}

func TestTemplateTransformer_ArrayExpansion_ComplexTemplate(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"combined": "{{.Username}}-{{.Attributes.groupList}}",
			"pure":     "{{.Attributes.groupList}}",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "john",
			Attributes: map[string]any{
				"groupList": []string{"admin", "users"},
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)

	// Complex template should remain as string
	assert.Equal(t, "john-[admin users]", transformed["john"].Attributes["combined"])

	// Pure array reference should expand
	pure, ok := transformed["john"].Attributes["pure"].([]string)
	require.True(t, ok, "pure should be []string, got %T", transformed["john"].Attributes["pure"])
	assert.Equal(t, []string{"admin", "users"}, pure)
}

func TestTemplateTransformer_ArrayExpansion_EmptyArray(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"groups": "{{.Attributes.groupList}}",
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "john",
			Attributes: map[string]any{
				"groupList": []string{},
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)

	groups, ok := transformed["john"].Attributes["groups"].([]string)
	require.True(t, ok, "groups should be []string, got %T", transformed["john"].Attributes["groups"])
	assert.Equal(t, []string{}, groups)
}

func TestTemplateTransformer_StringFunctions(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"upperName":  `{{.Username | upper}}`,
			"lowerEmail": `{{.Email | lower}}`,
			"trimmed":    `{{.Attributes.text | trim}}`,
			"replaced":   `{{.Username | replace "o" "0"}}`,
			"joined":     `{{.Attributes.items | join ","}}`,
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "john",
			Email:    "JOHN@EXAMPLE.COM",
			Attributes: map[string]any{
				"text":  "  hello  ",
				"items": []string{"a", "b", "c"},
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(t, "JOHN", transformed["john"].Attributes["upperName"])
	assert.Equal(t, "john@example.com", transformed["john"].Attributes["lowerEmail"])
	assert.Equal(t, "hello", transformed["john"].Attributes["trimmed"])
	assert.Equal(t, "j0hn", transformed["john"].Attributes["replaced"])
	assert.Equal(t, "a,b,c", transformed["john"].Attributes["joined"])
}

func TestTemplateTransformer_EncodingFunctions(t *testing.T) {
	t.Run("encode", func(t *testing.T) {
		config := map[string]any{
			"templates": map[string]any{
				"encoded": `{{.Username | b64enc}}`,
			},
		}

		tt, err := NewTemplateTransformer(config)
		require.NoError(t, err)

		identities := map[string]source.Identity{
			"john": {
				Username:   "john",
				Attributes: make(map[string]any),
			},
		}

		ctx := NewContext(context.Background(), nil)
		transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

		require.NoError(t, err)
		assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("john")), transformed["john"].Attributes["encoded"])
	})

	t.Run("decode", func(t *testing.T) {
		config := map[string]any{
			"templates": map[string]any{
				"decoded": `{{.Attributes.encoded | b64dec}}`,
			},
		}

		tt, err := NewTemplateTransformer(config)
		require.NoError(t, err)

		encodedSecret := base64.StdEncoding.EncodeToString([]byte("secret"))
		identities := map[string]source.Identity{
			"john": {
				Username: "john",
				Attributes: map[string]any{
					"encoded": encodedSecret,
				},
			},
		}

		ctx := NewContext(context.Background(), nil)
		transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

		require.NoError(t, err)
		assert.Equal(t, "secret", transformed["john"].Attributes["decoded"])
	})
}

func TestTemplateTransformer_JsonFunctions(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"json":       `{{.Attributes.data | toJson}}`,
			"prettyJson": `{{.Attributes.data | toPrettyJson}}`,
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "john",
			Attributes: map[string]any{
				"data": map[string]any{
					"name": "John",
					"age":  30,
				},
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"John","age":30}`, transformed["john"].Attributes["json"].(string))
	assert.Contains(t, transformed["john"].Attributes["prettyJson"], "John")
}

func TestTemplateTransformer_DefaultFunction(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"email":      `{{default "noreply@example.com" .Email}}`,
			"department": `{{default "Unknown" .Attributes.dept}}`,
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username:   "john",
			Email:      "",
			Attributes: map[string]any{},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(t, "noreply@example.com", transformed["john"].Attributes["email"])
	assert.Equal(t, "Unknown", transformed["john"].Attributes["department"])
}

func TestTemplateTransformer_TernaryFunction(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"status": `{{ternary "active" "inactive" (eq .Username "john")}}`,
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {Username: "john", Attributes: make(map[string]any)},
		"jane": {Username: "jane", Attributes: make(map[string]any)},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(t, "active", transformed["john"].Attributes["status"])
	assert.Equal(t, "inactive", transformed["jane"].Attributes["status"])
}

func TestTemplateTransformer_ListFunctions(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"firstGroup": `{{.Attributes.groups | first}}`,
			"lastGroup":  `{{.Attributes.groups | last}}`,
			"restGroups": `{{.Attributes.groups | rest | join ","}}`,
			"uniqueNums": `{{.Attributes.numbers | uniq}}`,
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "john",
			Attributes: map[string]any{
				"groups":  []string{"admin", "users", "developers"},
				"numbers": []int{1, 2, 2, 3, 3, 3},
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(t, "admin", transformed["john"].Attributes["firstGroup"])
	assert.Equal(t, "developers", transformed["john"].Attributes["lastGroup"])
	assert.Equal(t, "users,developers", transformed["john"].Attributes["restGroups"])
}

func TestTemplateTransformer_RegexFunctions(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"matches":   `{{regexMatch "^jo" .Username}}`,
			"sanitized": `{{regexReplaceAll "[^a-z]" "" .Attributes.text}}`,
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "john",
			Attributes: map[string]any{
				"text": "hello123world",
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(t, "true", transformed["john"].Attributes["matches"])
	assert.Equal(t, "helloworld", transformed["john"].Attributes["sanitized"])
}

func TestTemplateTransformer_QuoteFunctions(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"quoted":  `{{.Username | quote}}`,
			"squoted": `{{.Username | squote}}`,
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username:   "john",
			Attributes: make(map[string]any),
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(t, `"john"`, transformed["john"].Attributes["quoted"])
	assert.Equal(t, `'john'`, transformed["john"].Attributes["squoted"])
}

func TestTemplateTransformer_ComplexPipeline(t *testing.T) {
	config := map[string]any{
		"templates": map[string]any{
			"email": `{{.Username | lower | trimSuffix "_temp" }}@{{.Attributes.domain | default "example.com"}}`,
		},
	}

	tt, err := NewTemplateTransformer(config)
	require.NoError(t, err)

	identities := map[string]source.Identity{
		"john": {
			Username: "JOHN_temp",
			Attributes: map[string]any{
				"domain": "",
			},
		},
	}

	ctx := NewContext(context.Background(), nil)
	transformed, _, err := tt.Transform(ctx, identities, map[string]source.Group{})

	require.NoError(t, err)
	assert.Equal(t, "john@example.com", transformed["john"].Attributes["email"])
}
