# Deployment Service

Service to manage the CI/CD of ansonallard.com self hosted services.

## Start Service

1. Copy `.env.sample` to `.env`
2. Fill out all required values
3. Run `go build .`
4. Execute the binary file, `./deployment-service`

## Linux Firewall rules

Ensure that the linux host running the binary allows traffic from external hosts on the port.

Check this using:

```
sudo ufw status verbose
```

Add firewall rule using:

```
sudo ufw allow <port>>/tcp
sudo ufw reload
```
