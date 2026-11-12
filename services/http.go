package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// HTTPService представляет собой обертку над клиентом для выполнения HTTP-запросов.
type HTTPService struct {
	client  *http.Client
	baseURL string
	headers map[string]string
}

// NewHTTPService создает новый экземпляр HTTP-сервиса.
// Если timeout равен 0, устанавливается значение по умолчанию (10 секунд).
func NewHTTPService(baseURL string, timeout time.Duration) *HTTPService {
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &HTTPService{
		client: &http.Client{
			Timeout: timeout,
		},
		baseURL: baseURL,
		headers: make(map[string]string),
	}
}

// SetHeader добавляет или обновляет заголовок, который будет передаваться во всех запросах.
func (s *HTTPService) SetHeader(key, value string) {
	s.headers[key] = value
}

// Get выполняет GET-запрос. Можно передать queryParams (url.Values), они автоматически добавятся к URL.
func (s *HTTPService) Get(ctx context.Context, endpoint string, queryParams url.Values, result interface{}) error {
	fullURL := s.baseURL + endpoint

	if len(queryParams) > 0 {
		fullURL += "?" + queryParams.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return fmt.Errorf("ошибка создания GET запроса: %w", err)
	}

	return s.doRequest(req, result)
}

// PostJSON отправляет JSON-объект методом POST и декодирует ответ в result.
func (s *HTTPService) PostJSON(ctx context.Context, endpoint string, payload interface{}, result interface{}) error {
	return s.sendWithBody(ctx, http.MethodPost, endpoint, payload, result)
}

// PutJSON отправляет JSON-объект методом PUT и декодирует ответ в result.
func (s *HTTPService) PutJSON(ctx context.Context, endpoint string, payload interface{}, result interface{}) error {
	return s.sendWithBody(ctx, http.MethodPut, endpoint, payload, result)
}

// PatchJSON отправляет JSON-объект методом PATCH и декодирует ответ в result.
func (s *HTTPService) PatchJSON(ctx context.Context, endpoint string, payload interface{}, result interface{}) error {
	return s.sendWithBody(ctx, http.MethodPatch, endpoint, payload, result)
}

// Delete выполняет DELETE-запрос. Можно передать queryParams (url.Values).
func (s *HTTPService) Delete(ctx context.Context, endpoint string, queryParams url.Values, result interface{}) error {
	fullURL := s.baseURL + endpoint

	if len(queryParams) > 0 {
		fullURL += "?" + queryParams.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, fullURL, nil)
	if err != nil {
		return fmt.Errorf("ошибка создания DELETE запроса: %w", err)
	}

	return s.doRequest(req, result)
}

// sendWithBody вспомогательный метод для отправки запросов с JSON-телом (POST, PUT, PATCH).
func (s *HTTPService) sendWithBody(ctx context.Context, method, endpoint string, payload interface{}, result interface{}) error {
	fullURL := s.baseURL + endpoint

	var bodyReader io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("ошибка маршалинга JSON для %s: %w", method, err)
		}
		bodyReader = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("ошибка создания %s запроса: %w", method, err)
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return s.doRequest(req, result)
}

// doRequest выполняет общий цикл отправки запроса, применения заголовков и обработки ответа.
func (s *HTTPService) doRequest(req *http.Request, result interface{}) error {
	s.applyHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка выполнения HTTP запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("неуспешный статус код: %d, тело: %s", resp.StatusCode, string(bodyBytes))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("ошибка декодирования JSON ответа: %w", err)
		}
	}

	return nil
}

// applyHeaders проставляет накопленные заголовки в текущий запрос.
func (s *HTTPService) applyHeaders(req *http.Request) {
	for k, v := range s.headers {
		req.Header.Set(k, v)
	}
}