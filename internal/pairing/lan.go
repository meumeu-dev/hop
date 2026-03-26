package pairing

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const (
	lanBroadcastPort = 19876
	lanTCPPort       = 19877
	lanPrefix        = "HOP_PAIR:"
)

// StartLANServer broadcasts pair data on LAN and waits for a TCP response.
// Returns the remote PairData when a client connects.
func StartLANServer(code string, data *PairData) (*PairData, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	encrypted, err := Encrypt(jsonData, code)
	if err != nil {
		return nil, err
	}

	broadcastMsg := lanPrefix + encrypted

	// Start TCP listener for the response
	tcpListener, err := net.Listen("tcp", fmt.Sprintf(":%d", lanTCPPort))
	if err != nil {
		return nil, fmt.Errorf("impossible d'écouter sur le port TCP %d: %w", lanTCPPort, err)
	}
	defer tcpListener.Close()

	// Channel for result
	resultCh := make(chan *PairData, 1)
	errCh := make(chan error, 1)
	stopBroadcast := make(chan struct{})

	// Start UDP broadcast goroutine
	go func() {
		conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{
			IP:   net.IPv4bcast,
			Port: lanBroadcastPort,
		})
		if err != nil {
			errCh <- fmt.Errorf("erreur broadcast UDP: %w", err)
			return
		}
		defer conn.Close()

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		// Send immediately first
		conn.Write([]byte(broadcastMsg))

		for {
			select {
			case <-stopBroadcast:
				return
			case <-ticker.C:
				conn.Write([]byte(broadcastMsg))
			}
		}
	}()

	// Start TCP accept goroutine
	go func() {
		tcpListener.(*net.TCPListener).SetDeadline(time.Now().Add(2 * time.Minute))
		conn, err := tcpListener.Accept()
		if err != nil {
			errCh <- fmt.Errorf("timeout: pas de réponse reçue sur le LAN")
			return
		}
		defer conn.Close()

		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		respData, err := io.ReadAll(io.LimitReader(conn, 65536))
		if err != nil {
			errCh <- fmt.Errorf("erreur lecture réponse TCP: %w", err)
			return
		}

		decrypted, err := Decrypt(string(respData), code)
		if err != nil {
			errCh <- fmt.Errorf("déchiffrement réponse échoué (code incorrect ?)")
			return
		}

		var pairData PairData
		if err := json.Unmarshal(decrypted, &pairData); err != nil {
			errCh <- fmt.Errorf("données de pairing corrompues")
			return
		}

		resultCh <- &pairData
	}()

	select {
	case result := <-resultCh:
		close(stopBroadcast)
		return result, nil
	case err := <-errCh:
		close(stopBroadcast)
		return nil, err
	case <-time.After(2 * time.Minute):
		close(stopBroadcast)
		return nil, fmt.Errorf("timeout: pas de réponse reçue sur le LAN")
	}
}

// ConnectLAN listens for LAN broadcasts, decrypts with code, and sends response via TCP.
// Returns the remote PairData from the broadcast.
func ConnectLAN(code string, data *PairData) (*PairData, error) {
	// Listen for UDP broadcasts
	udpAddr := &net.UDPAddr{
		Port: lanBroadcastPort,
	}
	conn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("impossible d'écouter sur le port UDP %d: %w", lanBroadcastPort, err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(2 * time.Minute))

	buf := make([]byte, 65536)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return nil, fmt.Errorf("timeout: aucun broadcast LAN détecté")
			}
			return nil, fmt.Errorf("erreur lecture UDP: %w", err)
		}

		msg := string(buf[:n])
		if !strings.HasPrefix(msg, lanPrefix) {
			continue
		}

		encrypted := strings.TrimPrefix(msg, lanPrefix)
		decrypted, err := Decrypt(encrypted, code)
		if err != nil {
			// Wrong code or not our broadcast, keep listening
			continue
		}

		var serverData PairData
		if err := json.Unmarshal(decrypted, &serverData); err != nil {
			continue
		}

		// Found valid broadcast, send response via TCP
		jsonResp, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}

		encResp, err := Encrypt(jsonResp, code)
		if err != nil {
			return nil, err
		}

		tcpAddr := net.JoinHostPort(remoteAddr.IP.String(), fmt.Sprintf("%d", lanTCPPort))
		tcpConn, err := net.DialTimeout("tcp", tcpAddr, 5*time.Second)
		if err != nil {
			return nil, fmt.Errorf("erreur connexion TCP vers %s: %w", tcpAddr, err)
		}
		defer tcpConn.Close()

		tcpConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := tcpConn.Write([]byte(encResp)); err != nil {
			return nil, fmt.Errorf("erreur envoi réponse TCP: %w", err)
		}

		return &serverData, nil
	}
}

// StartLANServerWithTimeout is like StartLANServer but with a custom timeout.
// Used for fallback logic (try LAN briefly, then switch to worker).
func StartLANServerWithTimeout(code string, data *PairData, timeout time.Duration) (*PairData, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	encrypted, err := Encrypt(jsonData, code)
	if err != nil {
		return nil, err
	}

	broadcastMsg := lanPrefix + encrypted

	tcpListener, err := net.Listen("tcp", fmt.Sprintf(":%d", lanTCPPort))
	if err != nil {
		return nil, fmt.Errorf("impossible d'écouter sur le port TCP %d: %w", lanTCPPort, err)
	}
	defer tcpListener.Close()

	resultCh := make(chan *PairData, 1)
	errCh := make(chan error, 1)
	stopBroadcast := make(chan struct{})

	go func() {
		conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{
			IP:   net.IPv4bcast,
			Port: lanBroadcastPort,
		})
		if err != nil {
			errCh <- fmt.Errorf("erreur broadcast UDP: %w", err)
			return
		}
		defer conn.Close()

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		conn.Write([]byte(broadcastMsg))

		for {
			select {
			case <-stopBroadcast:
				return
			case <-ticker.C:
				conn.Write([]byte(broadcastMsg))
			}
		}
	}()

	go func() {
		tcpListener.(*net.TCPListener).SetDeadline(time.Now().Add(timeout))
		conn, err := tcpListener.Accept()
		if err != nil {
			errCh <- fmt.Errorf("timeout LAN")
			return
		}
		defer conn.Close()

		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		respData, err := io.ReadAll(io.LimitReader(conn, 65536))
		if err != nil {
			errCh <- fmt.Errorf("erreur lecture réponse TCP: %w", err)
			return
		}

		decrypted, err := Decrypt(string(respData), code)
		if err != nil {
			errCh <- fmt.Errorf("déchiffrement réponse échoué")
			return
		}

		var pairData PairData
		if err := json.Unmarshal(decrypted, &pairData); err != nil {
			errCh <- fmt.Errorf("données de pairing corrompues")
			return
		}

		resultCh <- &pairData
	}()

	select {
	case result := <-resultCh:
		close(stopBroadcast)
		return result, nil
	case err := <-errCh:
		close(stopBroadcast)
		return nil, err
	case <-time.After(timeout):
		close(stopBroadcast)
		return nil, fmt.Errorf("timeout LAN")
	}
}

