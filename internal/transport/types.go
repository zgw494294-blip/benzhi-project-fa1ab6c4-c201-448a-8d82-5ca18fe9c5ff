package transport

import (
	"embed"
	"net/http"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/calibration"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/certificate"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/measurement"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/review"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
)

//go:embed web/*
var webFiles embed.FS

type Server struct {
	calibrations *calibration.Service
	measurements *measurement.Service
	reviews      *review.Service
	certificates *certificate.Service
	store        *store.Store
	mux          *http.ServeMux
}
