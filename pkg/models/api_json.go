package models

// HealthOKBody documents GET /api/v1/ 200 JSON (standard success envelope).
type HealthOKBody struct {
	Data string `json:"data"`
}

// LoginAPIResponse documents POST /login 200 JSON.
type LoginAPIResponse struct {
	Data LoginTokenBody `json:"data"`
}

// RegisterAPIResponse documents POST /register 201 JSON.
type RegisterAPIResponse struct {
	Data RegisterSuccessBody `json:"data"`
}
