# Doorman analytics

I needed simple analytics for my blog, preferably something lightweight that doesn't need a clunky postgres or mysql database because memory is expensive

# How to use

1. Add to your website:

```javascript
<script src="http://your-domain.com:8080/assets/js/t.js" async></script>
```

2. Access dashboard:
   http://localhost:8080/login
   Default credentials: admin / admin123

3. With [kamal](https://kamal-deploy.org/)

```yaml
accessories:
  doorman:
    image: ghcr.io/webbesoft/doorman:latest
    host: your-host-ip
    proxy:
      ssl: true
      host: your-host.com
      app_port: 8080
    env:
      secret:
        - ADMIN_USER
        - ADMIN_PASSWORD
        - DOORMAN_SESSION_SECRET
    port: "8080:8080"
    volumes:
      - analytics.db:/app/analytics.db
```

4. Docker compose

```yaml
services:
  doorman:
    image: ghcr.io/webbesoft/doorman:latest
    environment:
      - ADMIN_USER
      - ADMIN_PASSWORD
      - DOORMAN_SESSION_SECRET
    port: "8080:8080"
    volumes:
      - analytics.db:/app/analytics.db
```

GDPR Compliance Features:

- No cookies used for tracking
- IP addresses are hashed (SHA256)
- No personal data stored
- Minimal data collection (URL, referrer only)
- No cross-site tracking
- No persistent identifiers
