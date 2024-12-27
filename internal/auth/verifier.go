package auth

type Verifier struct {
	jwtSecret     []byte
	adminPassword string
}
