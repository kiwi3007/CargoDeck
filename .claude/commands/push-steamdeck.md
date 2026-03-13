Build and deploy the Playerr agent binary to the Steam Deck.

Run the following steps:
1. Build the Linux agent binary:
   `export GOROOT=/home/kieran/go GOPATH=/home/kieran/gopath PATH=$PATH:/home/kieran/go/bin && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o /tmp/playerr-agent-linux ./cmd/agent/`
2. Copy to Steam Deck:
   `scp -i ~/.ssh/id_ed25519 /tmp/playerr-agent-linux deck@192.168.4.111:/tmp/playerr-agent-new`
3. Install and restart the service on Steam Deck:
   `ssh -i ~/.ssh/id_ed25519 deck@192.168.4.111 'systemctl --user stop playerr-agent; mv /tmp/playerr-agent-new ~/.config/playerr-agent/playerr-agent; chmod +x ~/.config/playerr-agent/playerr-agent; systemctl --user start playerr-agent; sleep 2; systemctl --user status playerr-agent --no-pager'`
4. Report the service status output.
