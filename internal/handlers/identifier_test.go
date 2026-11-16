package handlers

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestCreateUsesThingIDTextAsCaguarIdentifier(t *testing.T) {
	app := fiber.New()
	app.Post("/dispatch/resources", func(c *fiber.Ctx) error {
		adoptCaguarIdentifier(c)
		return c.Send(c.Body())
	})
	req := httptest.NewRequest(fiber.MethodPost, "/dispatch/resources", strings.NewReader(`{"thing_id":"PHONE-1122","unique_id":"UNIT1122","name":"Engine 1122"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	text := string(body)
	if strings.Contains(text, "thing_id") || !strings.Contains(text, `"unique_id":"PHONE-1122"`) {
		t.Fatalf("body %s", text)
	}
}
