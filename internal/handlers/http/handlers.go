package httphandlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/alash3al/stash/internal/memory"
	"github.com/labstack/echo/v5"
)

// Health is a simple health check handler.
func Health(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// AddFact returns a handler that remembers events.
func AddFact(bc *bootstrap.Context) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var req struct {
			Content    string            `json:"content"`
			Confidence float64           `json:"confidence"`
			Metadata   map[string]string `json:"metadata,omitempty"`
		}
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
		}

		if strings.TrimSpace(req.Content) == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "content is required"})
		}

		namespace := c.QueryParam("namespace")
		metadata := make(map[string]any)
		if req.Metadata != nil {
			for k, v := range req.Metadata {
				metadata[k] = v
			}
		}

		eventID, err := bc.Memory.Remember(c.Request().Context(), namespace, req.Content, metadata)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		return c.JSON(http.StatusCreated, map[string]string{
			"id":      eventID,
			"message": "Event remembered successfully",
		})
	}
}

// RecallFacts returns a handler that recalls facts by query.
func RecallFacts(bc *bootstrap.Context) echo.HandlerFunc {
	return func(c *echo.Context) error {
		query := c.QueryParam("query")
		if strings.TrimSpace(query) == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "query is required"})
		}

		namespace := c.QueryParam("namespace")
		limit := 10
		if l := c.QueryParam("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}

		ranked := c.QueryParam("ranked") == "true"

		var facts []memory.Fact
		var err error

		if ranked {
			facts, err = bc.Memory.RecallFactsRanked(c.Request().Context(), namespace, query, limit)
		} else {
			facts, err = bc.Memory.QueryFactsByType(c.Request().Context(), namespace, "state")
		}

		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		type factResponse struct {
			ID               string  `json:"id"`
			Content          string  `json:"content"`
			Type             string  `json:"type"`
			Confidence       float32 `json:"confidence"`
			ObservationCount int     `json:"observation_count"`
			Source           string  `json:"source"`
			Score            float32 `json:"score,omitempty"`
			ValidFrom        string  `json:"valid_from"`
		}

		output := make([]factResponse, len(facts))
		for i, f := range facts {
			output[i] = factResponse{
				ID:               f.ID,
				Content:          f.Content,
				Type:             f.Type,
				Confidence:       f.Confidence,
				ObservationCount: f.ObservationCount,
				Source:           f.Source,
				Score:            f.Score,
				ValidFrom:        f.ValidFrom.Format("2006-01-02T15:04:05Z"),
			}
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"query":     query,
			"namespace": namespace,
			"ranked":    ranked,
			"limit":     limit,
			"facts":     output,
		})
	}
}

// RegisterRoutes wires all handlers to the Echo instance.
func RegisterRoutes(e *echo.Echo, bc *bootstrap.Context) {
	e.POST("/api/v1/facts", AddFact(bc))
	e.GET("/api/v1/facts", RecallFacts(bc))
	e.GET("/health", Health)
}
