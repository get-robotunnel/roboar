package server

import (
	"errors"
	"net/http"

	"github.com/RussellTNY/robot-agent-registry/internal/auth"
	"github.com/RussellTNY/robot-agent-registry/internal/ids"
	"github.com/RussellTNY/robot-agent-registry/internal/model"
	"github.com/RussellTNY/robot-agent-registry/internal/store"
	"github.com/gin-gonic/gin"
)

type createOwnerReq struct {
	PublicKey   string `json:"public_key"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

func (s *Server) createOwner(c *gin.Context) {
	var req createOwnerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.DisplayName == "" {
		abort(c, http.StatusBadRequest, "display_name is required")
		return
	}
	if err := auth.ValidatePublicKey(req.PublicKey); err != nil {
		abort(c, http.StatusBadRequest, "public_key must be a 32-byte Ed25519 key in hex")
		return
	}

	owner := &model.Owner{
		OwnerID:     ids.Owner(),
		PublicKey:   req.PublicKey,
		DisplayName: req.DisplayName,
		Email:       req.Email,
	}
	if err := s.store.CreateOwner(c, owner); err != nil {
		if isUniqueViolation(err) {
			abort(c, http.StatusConflict, "public_key already registered")
			return
		}
		abort(c, http.StatusInternalServerError, "could not create owner")
		return
	}
	c.JSON(http.StatusCreated, owner)
}

type challengeReq struct {
	PublicKey string `json:"public_key"`
}

func (s *Server) authChallenge(c *gin.Context) {
	var req challengeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid body")
		return
	}
	if _, err := s.store.GetOwnerByPublicKey(c, req.PublicKey); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			abort(c, http.StatusNotFound, "no owner registered for this public_key")
			return
		}
		abort(c, http.StatusInternalServerError, "lookup failed")
		return
	}
	challenge, err := s.auth.NewChallenge(req.PublicKey)
	if err != nil {
		abort(c, http.StatusBadRequest, "invalid public_key")
		return
	}
	c.JSON(http.StatusOK, gin.H{"challenge": challenge, "expires_in": 300})
}

type verifyReq struct {
	PublicKey string `json:"public_key"`
	Challenge string `json:"challenge"`
	Signature string `json:"signature"`
}

func (s *Server) authVerify(c *gin.Context) {
	var req verifyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.auth.VerifyChallenge(req.PublicKey, req.Challenge, req.Signature); err != nil {
		abort(c, http.StatusUnauthorized, err.Error())
		return
	}
	owner, err := s.store.GetOwnerByPublicKey(c, req.PublicKey)
	if err != nil {
		abort(c, http.StatusInternalServerError, "lookup failed")
		return
	}
	token, err := s.auth.IssueJWT(owner.OwnerID)
	if err != nil {
		abort(c, http.StatusInternalServerError, "could not issue token")
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "owner_id": owner.OwnerID, "expires_in": 86400})
}

func (s *Server) getMe(c *gin.Context) {
	ownerID := c.GetString(ctxOwnerID)
	owner, err := s.store.GetOwnerByID(c, ownerID)
	if err != nil {
		abort(c, http.StatusNotFound, "owner not found")
		return
	}
	c.JSON(http.StatusOK, owner)
}
