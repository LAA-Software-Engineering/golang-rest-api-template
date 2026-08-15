package models

// HealthOKBody documents GET /api/v1/ 200 JSON (standard success envelope).
type HealthOKBody struct {
	Data string `json:"data"`
}

// LoginAPIResponse documents POST /login 200 JSON.
type LoginAPIResponse struct {
	Data LoginTokenBody `json:"data"`
}

// RefreshAPIResponse documents POST /refresh 200 JSON.
type RefreshAPIResponse struct {
	Data LoginTokenBody `json:"data"`
}

// LogoutAPIResponse documents POST /logout 200 JSON.
type LogoutAPIResponse struct {
	Data LogoutSuccessBody `json:"data"`
}

// RegisterAPIResponse documents POST /register 201 JSON.
type RegisterAPIResponse struct {
	Data RegisterSuccessBody `json:"data"`
}

// AdminMeAPIResponse documents GET /admin/me 200 JSON.
type AdminMeAPIResponse struct {
	Data AdminMeBody `json:"data"`
}
