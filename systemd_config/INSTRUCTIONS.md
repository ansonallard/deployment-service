## Running Binary with `systemd`

1. Copy `deployment-service.service.example` to `/etc/systemd/system/deployment-service.service` and populate the appropriate values
2. Run the following commands:

```
# Make sure your binary is executable
chmod +x /path/to/your/deployment-service/deployment-service

# Secure your .env file (contains sensitive data)
chmod 600 /path/to/your/deployment-service/.env
chown your-username:your-username /path/to/your/deployment-service/.env

# Reload systemd to recognize the new service
sudo systemctl daemon-reload

# Enable the service to start on boot
sudo systemctl enable deployment-service

# Start the service now
sudo systemctl start deployment-service

# Check the status
sudo systemctl status deployment-service
```
