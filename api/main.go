package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Peer struct {
	ID        string    `json:"id" bson:"id"`
	Name      string    `json:"name" bson:"name"`
	PublicKey string    `json:"public_key" bson:"public_key"`
	AllowedIP string    `json:"allowed_ip" bson:"allowed_ip"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

type CreatePeerRequest struct {
	Name string `json:"name"`
}

var collection *mongo.Collection

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func randomID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func nextIP(ctx context.Context) string {
	count, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		count = 0
	}
	// Simple allocator for the starter project: 10.8.0.2 onward.
	return fmt.Sprintf("10.8.0.%d/32", count+2)
}

func run(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func addPeer(p Peer, privateKey string) error {
	// The API container needs access to the host WireGuard control socket.
	// For a production deployment, replace this with a privileged helper
	// or a dedicated host-side agent.
	return run("wg", "set", env("VPN_INTERFACE", "wg0"),
		"peer", p.PublicKey, "allowed-ips", p.AllowedIP)
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func createPeer(w http.ResponseWriter, r *http.Request) {
	var req CreatePeerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	privateOut, err := exec.Command("wg", "genkey").Output()
	if err != nil {
		http.Error(w, "failed to generate key", 500)
		return
	}
	privateKey := strings.TrimSpace(string(privateOut))

	pubCmd := exec.Command("wg", "pubkey")
	pubCmd.Stdin = strings.NewReader(privateKey)
	publicOut, err := pubCmd.Output()
	if err != nil {
		http.Error(w, "failed to generate public key", 500)
		return
	}
	publicKey := strings.TrimSpace(string(publicOut))

	p := Peer{
		ID:        randomID(),
		Name:      req.Name,
		PublicKey: publicKey,
		AllowedIP: mustNextIP(ctx),
		CreatedAt: time.Now().UTC(),
	}

	if err := addPeer(p, privateKey); err != nil {
		http.Error(w, "failed to add WireGuard peer: "+err.Error(), 500)
		return
	}

	if _, err := collection.InsertOne(ctx, p); err != nil {
		http.Error(w, "failed to store peer: "+err.Error(), 500)
		return
	}

	publicEndpoint := env("VPN_PUBLIC_ENDPOINT", "YOUR_PUBLIC_IP_OR_DNS")
	port := env("VPN_PORT", "12345")
	serverPub := getServerPublicKey()

	config := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s
DNS = 1.1.1.1

[Peer]
PublicKey = %s
Endpoint = %s:%s
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
`, privateKey, strings.TrimSuffix(p.AllowedIP, "/32")+"/24", serverPub, publicEndpoint, port)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"peer":   p,
		"config": config,
	})
}

func mustNextIP(ctx context.Context) string {
	return nextIP(ctx)
}

func getServerPublicKey() string {
	data, err := os.ReadFile("/etc/wireguard/server_public.key")
	if err != nil {
		return "SERVER_PUBLIC_KEY"
	}
	return strings.TrimSpace(string(data))
}

func listPeers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cur, err := collection.Find(ctx, bson.M{})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer cur.Close(ctx)

	var peers []Peer
	if err := cur.All(ctx, &peers); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(peers)
}

func main() {
	uri := os.Getenv("MONGODB_URI")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		panic(err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		panic(err)
	}

	collection = client.Database("vpn").Collection("peers")

	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/peers", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listPeers(w, r)
		case http.MethodPost:
			createPeer(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})

	s := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Println("VPN management API listening on :8080")
	if err := s.ListenAndServe(); err != nil {
		panic(err)
	}
}
