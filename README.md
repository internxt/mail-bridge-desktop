1. We need to create a self-signed certificate for connecting to Apple Mail: 
  
    1.1 Create a self-signed certificate with SAN `san.cnf` by running: `openssl req -x509 -newkey rsa:2048 -keyout key.pem -out cert.pem -days 365 -nodes -config san.cnf`
 
    1.2  Mark it as trusted by the system: `sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain cert.pem`
 
2. Run `npm init -y` and `npm install` to install the dependencies of `crypto-bridge.mjs`
3. Install all required modules for Python. Better in a separate environment: 
```
python3 -m venv .venv
source .venv/bin/activate
python3 -m pip install cryptography requests
```
4. Run as administrator (because we need system ports) `sudo python3 mail_server.py`
5. Add a new account to Apple Mail with email and password. It will fail because the mail server is unknown. Put localhost in both fields:
