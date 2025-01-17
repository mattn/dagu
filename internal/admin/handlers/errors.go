package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/yohamta/dagu/internal/dag"
)

var (
	errInvalidArgs = errors.New("invalid argument")
	errNotFound    = errors.New("not found")
)

func formatError(err error) string {
	return fmt.Sprintf("[Error] %s", err.Error())
}

func encodeError(w http.ResponseWriter, err error) {
	switch err {
	case dag.ErrDAGNotFound:
		http.Error(w, formatError(err), http.StatusNotFound)
	case errInvalidArgs:
		http.Error(w, formatError(err), http.StatusBadRequest)
	case errNotFound:
		http.Error(w, formatError(err), http.StatusNotFound)
	default:
		http.Error(w, formatError(err), http.StatusInternalServerError)
	}
}
