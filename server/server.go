package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"wtt/common"

	"github.com/cornelk/hashmap"
	"github.com/go-chi/chi/v5"
	"github.com/pion/webrtc/v4"
)

type MessageChannel struct {
	offer  chan webrtc.SessionDescription
	answer chan webrtc.SessionDescription
}

var hostM = hashmap.New[string, MessageChannel]()

func Run(ctx context.Context, listenAddr string, tokens []string, maxMsgSize int64) <-chan error {
	slog.Info("server starting", "listen", listenAddr)

	ec := make(chan error, 1)

	router := chi.NewRouter()
	router.Use(LimitRequestBodySize(maxMsgSize))

	router.Post("/"+string(common.RTCRegisterType), register)
	router.Post("/"+string(common.RTCOfferType), receiveOffer)
	router.Get("/"+string(common.RTCOfferType)+"/{hostID}", sendOffer)
	router.Post("/"+string(common.RTCAnswerType), receiveAnswer)
	router.Get("/"+string(common.RTCAnswerType)+"/{hostID}", sendAnswer)

	srv := &http.Server{Addr: listenAddr, Handler: router}

	go func() {
		<-ctx.Done()
		slog.Info("server context cancelled, shutting down")
		_ = srv.Shutdown(context.Background())
	}()

	go func() {
		slog.Info("server listening", "addr", listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			ec <- err
			return
		}
		slog.Info("server exited")
		ec <- nil
	}()

	return ec
}

func register(w http.ResponseWriter, r *http.Request) {
	var msg common.RTCRegister
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		slog.Error("decode register message error", "err", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	slog.Info("received register message", "id", msg.HostID)

	hostM.Set(msg.HostID, MessageChannel{
		offer:  make(chan webrtc.SessionDescription),
		answer: make(chan webrtc.SessionDescription),
	})

	w.WriteHeader(http.StatusOK)
}

func receiveOffer(w http.ResponseWriter, r *http.Request) {
	var offer common.RTCOffer
	if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
		slog.Error("decode offer message error", "err", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	slog.Info("received offer message", "id", offer.HostID)

	c, ok := hostM.Get(offer.HostID)
	if !ok {
		slog.Error("host not found", "id", offer.HostID)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	c.offer <- offer.SessionDescription

	w.WriteHeader(http.StatusOK)
}

func sendOffer(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostID")

	c, ok := hostM.Get(hostID)
	if !ok {
		slog.Error("host not found", "id", hostID)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	offer := <-c.offer

	offerJ, err := json.Marshal(offer)
	if err != nil {
		slog.Error("encode offer message error", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Write(offerJ)
}

func receiveAnswer(w http.ResponseWriter, r *http.Request) {
	var answer common.RTCAnswer
	if err := json.NewDecoder(r.Body).Decode(&answer); err != nil {
		slog.Error("decode answer message error", "err", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	slog.Info("received answer message")

	c, ok := hostM.Get(answer.HostID)
	if !ok {
		slog.Error("host not found", "id", answer.HostID)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	c.answer <- answer.SessionDescription

	w.WriteHeader(http.StatusOK)
}

func sendAnswer(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostID")

	c, ok := hostM.Get(hostID)
	if !ok {
		slog.Error("host not found", "id", hostID)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	answer := <-c.answer

	answerJ, err := json.Marshal(answer)
	if err != nil {
		slog.Error("encode answer message error", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Write(answerJ)
}
