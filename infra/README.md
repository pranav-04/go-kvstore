# Azure VM Deployment

This document describes how to deploy the KV store on a Linux VM on Azure with:

- Go HTTP server
- Nginx reverse proxy
- systemd process management
- Azure VM networking

Planning to automate most of this in future.

## Architecture

```text
                         Internet
                            |
                            | TCP :80
                            v
                  +--------------------+
                  |    Azure VM        |
                  |                    |
                  |   Nginx :80        |
                  |      |             |
                  |      | HTTP        |
                  |      v             |
                  |   Go :8080         |
                  |  127.0.0.1        |
                  |                    |
                  |     systemd        |
                  +--------------------+
```

The Go server is bound to `127.0.0.1:8080`, so it is not directly accessible from the Internet. Nginx is the public-facing HTTP server and reverse proxies requests to the Go application.

## 1. Create an Azure VM

Create a resource group:

```bash
az group create \
  --name go-kvstore-rg \
  --location <location>
```

Create the VM:

```bash
az vm create \
  --resource-group go-kvstore-rg \
  --name go-kvstore-vm \
  --image Ubuntu2404 \
  --size Standard_B2s \
  --admin-username azureuser \
  --generate-ssh-keys
```

Get the VM's public IP:

```bash
az vm show \
  --resource-group go-kvstore-rg \
  --name go-kvstore-vm \
  --show-details \
  --query publicIps \
  -o tsv
```

SSH into the VM:

```bash
ssh azureuser@<PUBLIC_IP>
```

## 2. Install dependencies

Update the package index:

```bash
sudo apt update
```

Install Git and Go:

```bash
sudo apt install git golang-go -y
```

Install Nginx:

```bash
sudo apt install nginx -y
```

Verify:

```bash
git --version
go version
nginx -v
```

## 3. Clone the repository

Clone the repository:

```bash
git clone <GITHUB_REPOSITORY>
cd <REPOSITORY>
```

## 4. Application directories

Runtime application files are kept separately from the source repository:

```text
/opt/kvstore/
├── bin/
│   └── kvstore-server
└── data/
    └── kvstore.db
```

Create the directories:

```bash
sudo mkdir -p /opt/kvstore/bin
sudo mkdir -p /opt/kvstore/data
```

Give the application user ownership:

```bash
sudo chown -R azureuser:azureuser /opt/kvstore
```

The repository contains source code, while `/opt/kvstore/data` contains runtime state.

## 5. Configure the Go server

The Go HTTP server listens only on localhost:

```go
http.ListenAndServe("127.0.0.1:8080", handler)
```

This prevents external clients from connecting directly to the application.

The network path is:

```text
Client
  |
  | :80
  v
Nginx
  |
  | 127.0.0.1:8080
  v
Go server
```

## 6. Build the application

From the repository:

```bash
go build -o /opt/kvstore/bin/kvstore-server ./cmd/server
```

Verify:

```bash
ls -l /opt/kvstore/bin/
```

## 7. Configure systemd

Create:

```bash
sudo nano /etc/systemd/system/kvstore-server.service
```

Use:

```ini
[Unit]
Description=KV Store Server
After=network.target

[Service]
Type=simple
User=azureuser
ExecStart=/opt/kvstore/bin/kvstore-server
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

Reload systemd:

```bash
sudo systemctl daemon-reload
```

Start the service:

```bash
sudo systemctl start kvstore-server
```

Enable it at boot:

```bash
sudo systemctl enable kvstore-server
```

Check status:

```bash
sudo systemctl status kvstore-server
```

View logs:

```bash
sudo journalctl -u kvstore-server
```

Follow logs:

```bash
sudo journalctl -u kvstore-server -f
```

### Verify the Go server

Check the listening socket:

```bash
sudo ss -lntp | grep 8080
```

Expected:

```text
127.0.0.1:8080
```

Test locally:

```bash
curl http://127.0.0.1:8080
```

## 8. Configure Nginx

Create:

```bash
sudo nano /etc/nginx/sites-available/kvstore
```

Configuration:

```nginx
server {
    listen 80;

    server_name _;

    location / {
        proxy_pass http://127.0.0.1:8080;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Remove the default configuration:

```bash
sudo rm /etc/nginx/sites-enabled/default
```

Enable the KV store configuration:

```bash
sudo ln -s \
  /etc/nginx/sites-available/kvstore \
  /etc/nginx/sites-enabled/kvstore
```

Test the configuration:

```bash
sudo nginx -t
```

Reload Nginx:

```bash
sudo systemctl reload nginx
```

## 9. Verify Nginx

Check Nginx:

```bash
sudo systemctl status nginx
```

Check the listening sockets:

```bash
sudo ss -lntp
```

Expected:

```text
0.0.0.0:80       nginx
127.0.0.1:8080   kvstore-server
```

From your local machine:

```bash
curl http://<PUBLIC_IP>
```

The request path is:

```text
Local machine
      |
      | HTTP :80
      v
Azure Public IP
      |
      v
Nginx
      |
      | HTTP :8080
      v
Go KV store
```

## 10. Useful debugging commands

### Network sockets

```bash
sudo ss -lntp
```

### Running processes

```bash
ps aux | grep kvstore
ps aux | grep nginx
```

### KV store logs

```bash
sudo journalctl -u kvstore-server -f
```

### Nginx access logs

```bash
sudo tail -f /var/log/nginx/access.log
```

### Nginx error logs

```bash
sudo tail -f /var/log/nginx/error.log
```

### Full Nginx configuration

```bash
sudo nginx -T
```

### Test Go directly

```bash
curl http://127.0.0.1:8080
```

### Test through Nginx

```bash
curl http://<PUBLIC_IP>
```

## 11. Process failure test

systemd is configured to restart the application automatically.

Find the process:

```bash
ps aux | grep kvstore-server
```

Kill it:

```bash
sudo pkill kvstore-server
```

Check:

```bash
sudo systemctl status kvstore-server
```

systemd should start a replacement process automatically.

## 12. Network traffic inspection

To observe traffic at the network level:

```bash
sudo tcpdump -i any port 80 or port 8080
```

Then make a request:

```bash
curl http://<PUBLIC_IP>
```

This allows you to observe the two connections:

```text
Client -> Nginx :80

Nginx -> Go :8080
```

## 13. Updating the application

Pull the latest code:

```bash
cd <REPOSITORY>
git pull
```

Build the new version:

```bash
go build -o /opt/kvstore/bin/kvstore-server ./cmd/server
```

Restart the service:

```bash
sudo systemctl restart kvstore-server
```

Verify:

```bash
sudo systemctl status kvstore-server
```

## Cleanup

To delete all Azure resources created for this exercise:

```bash
az group delete \
  --name go-kvstore-rg \
  --yes \
  --no-wait
```

## Troubleshooting

For service startup failures check

```bash
sudo journalctl -u kvstore-server -n 50 --no-pager
```