# Custom docker image

This gitea build have base to run gitea, bases is `debian:sid`, installed software:

- Git
- Gnupg
- Gitea

## Run

Docker:

```sh
docker run --restart on-failure --pull always -d -v gitea_data:/data -p 3000:3000 sirherobrine23.com.br/gitea/gitea:latest
```

Docker compose:

```yml
name: "Gitea server"

volumes:
  # SSL Certs
  ssl:
    labels: [ "ssl", "certs" ]
  # Data to storage all gitea data, includes config and repositorys
  gitdata:
  dbdata:

# make default network communication
#
# CIDR to IPv4: 10.4.0.0/24
# CIDR to IPv6: fd0f:df83:64a7::/48
#
# Gitea:    10.4.0.2, fd0f:df83:64a7::0002
# Postgres: 10.4.0.3, fd0f:df83:64a7::0003
# Redis:    10.4.0.4, fd0f:df83:64a7::0004
networks:
  sh23_services:
    name: sh23_services
    enable_ipv6: true
    driver: bridge
    ipam:
      driver: default
      config:
        - subnet: 10.4.0.0/24
          gateway: 10.4.0.1
        - subnet: fd0f:df83:64a7::/48
          gateway: fd0f:df83:64a7::0001

services:
  # Build gitea image and deploy config
  gitea:
    container_name: gitea
    restart: "always"
    image: sirherobrine23.com.br/gitea/gitea:latest
    pull_policy: always
    depends_on:
      - db
      - redis
    env_file:
      - .env
    environment:
      GITEA__database__USER: ${DB_USER}
      GITEA__database__PASSWD: ${DB_PASS}
      GITEA__mailer__ENABLED: "false"
      GITEA__email_0x2E_incoming__ENABLED: "false"
      GITEA__service__REGISTER_EMAIL_CONFIRM: "false"
    networks:
      sh23_services:
        ipv4_address: 10.4.0.2
        ipv6_address: fd0f:df83:64a7::0002
    ports:
      - 22:22/tcp   # git ssh
      - 80:80/tcp   # HTTP
      - 443:443/tcp # HTTPs
    volumes:
      - ssl:/ssl:ro             # SSL Certs read only
      - gitdata:/data:rw        # Storage all data here
      - ./app.ini:/data/app.ini # Config

  # postgres DB server
  db:
    image: postgres:17
    restart: always
    volumes:
      - dbdata:/var/lib/postgresql/data
    env_file:
      - .env
    environment:
      POSTGRES_PASSWORD: ${DB_PASS}
      POSTGRES_USER: ${DB_USER}
      POSTGRES_DB: ${DB_NAME}
    networks:
      sh23_services:
        ipv4_address: 10.4.0.3
        ipv6_address: fd0f:df83:64a7::0003
        aliases:
          - postgres

  # Start redis server
  redis:
    image: redis:latest
    restart: always
    mem_limit: 2000M
    networks:
      sh23_services:
        ipv4_address: 10.4.0.4
        ipv6_address: fd0f:df83:64a7::0004
```