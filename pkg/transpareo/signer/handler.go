package signer

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// MaxBodyBytes bounds a request body. The platform caps what it
// reads of the answer at the same size.
const MaxBodyBytes = 1 << 20

// HandlerOptions configure the HTTP handler.
type HandlerOptions struct {
	// Path is the one route served; every other path answers 404.
	Path string

	// Logger takes one line per request; nil discards them.
	Logger *slog.Logger
}

// Handler serves the signing route: it verifies the platform's
// signature, tells the two request shapes apart by their
// cryptosuite, and answers JSON. Refusals carry {"error": ...}.
func Handler(s *Signer, v *Verifier, opts HandlerOptions) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &handler{signer: s, verifier: v, path: opts.Path, logger: logger}
}

type handler struct {
	signer   *Signer
	verifier *Verifier
	path     string
	logger   *slog.Logger
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	if r.URL.Path != h.path {
		h.refuse(w, r, http.StatusNotFound, "no such route", started)
		return
	}
	if r.Method != http.MethodPost {
		h.refuse(w, r, http.StatusMethodNotAllowed, "POST only", started)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			h.refuse(w, r, http.StatusRequestEntityTooLarge,
				"body over 1 MiB", started)
			return
		}
		h.refuse(w, r, http.StatusBadRequest, "unreadable body", started)
		return
	}
	if err := h.verifier.Verify(r, body); err != nil {
		h.refuse(w, r, http.StatusUnauthorized, err.Error(), started)
		return
	}
	kind, resp, err := h.sign(body)
	if err != nil {
		h.refuse(w, r, http.StatusUnprocessableEntity, err.Error(), started)
		return
	}
	h.logger.Info("signed", "kind", kind, "elapsed",
		time.Since(started).Round(time.Millisecond))
	writeJSON(w, http.StatusOK, resp)
}

// sign tells the shapes apart: a base proof names its
// cryptosuite, a documents request carries snapshots.
func (h *handler) sign(body []byte) (string, any, error) {
	var probe struct {
		Cryptosuite string          `json:"cryptosuite"`
		Snapshots   json.RawMessage `json:"snapshots"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&probe); err != nil {
		return "", nil, errors.New("the body is not JSON")
	}
	switch {
	case probe.Cryptosuite == CryptosuiteBaseProof:
		var req BaseProofRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return "", nil, errors.New("malformed base proof request")
		}
		resp, err := h.signer.SignBaseProof(&req)
		return "base proof", resp, err
	case len(probe.Snapshots) > 0:
		var req DocumentsRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return "", nil, errors.New("malformed documents request")
		}
		resp, err := h.signer.SignDocuments(&req)
		return "documents", resp, err
	case probe.Cryptosuite != "":
		return "", nil, errors.New("unsupported cryptosuite " + probe.Cryptosuite)
	}
	return "", nil, errors.New("neither snapshots nor a base proof")
}

func (h *handler) refuse(w http.ResponseWriter, r *http.Request, status int,
	reason string, started time.Time) {
	h.logger.Warn("refused", "status", status, "reason", reason,
		"method", r.Method, "path", r.URL.Path, "elapsed",
		time.Since(started).Round(time.Millisecond))
	writeJSON(w, status, map[string]string{"error": reason})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
