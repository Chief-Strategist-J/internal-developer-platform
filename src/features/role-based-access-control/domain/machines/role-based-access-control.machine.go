package machines

type TokenState string

const (
	TokenStateUnauthenticated TokenState = "UNAUTHENTICATED"
	TokenStateActive          TokenState = "ACTIVE"
	TokenStateRevoked         TokenState = "REVOKED"
	TokenStateExpired         TokenState = "EXPIRED"
)

type TokenTransitionEvent string

const (
	EventAuthenticate TokenTransitionEvent = "AUTHENTICATE"
	EventRevoke       TokenTransitionEvent = "REVOKE"
	EventExpire       TokenTransitionEvent = "EXPIRE"
)

type TokenLifecycleMachine struct {
	currentState TokenState
}

func NewTokenLifecycleMachine(initialState TokenState) *TokenLifecycleMachine {
	if initialState == "" {
		initialState = TokenStateUnauthenticated
	}
	return &TokenLifecycleMachine{currentState: initialState}
}

func (m *TokenLifecycleMachine) CurrentState() TokenState {
	return m.currentState
}

func (m *TokenLifecycleMachine) Transition(event TokenTransitionEvent) (TokenState, bool) {
	switch m.currentState {
	case TokenStateUnauthenticated:
		if event == EventAuthenticate {
			m.currentState = TokenStateActive
			return m.currentState, true
		}
	case TokenStateActive:
		if event == EventRevoke {
			m.currentState = TokenStateRevoked
			return m.currentState, true
		}
		if event == EventExpire {
			m.currentState = TokenStateExpired
			return m.currentState, true
		}
	case TokenStateRevoked, TokenStateExpired:
		return m.currentState, false
	}
	return m.currentState, false
}
