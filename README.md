# githooker
A minimial HTTP server to respond to github hooks and run arbitrary shell scripts in response.

# environment variables
variable | default | description
--- | --- | ---
GH_LISTEN_PORT | 4040 | port used by HTTP server
GH_HMAC_KEY | | secret key set in github hook configuration
GH_CMD_ROOT | /etc/githooker | root of hook command folder structure
GH_MAX_RUN_SECS | 90 | maximum runtime of hook command (in seconds)
GH_CMD_EXTENSIONS | | space-separated list of file extensions to try for hook command

# installation
1) Clone and build the githooker executable file.
2) Configure a HTTP path to the githooker listener. Rather than expose it directly to the internet, I recommend using a reverse-proxy like `nginx` and an obscure path (I tend to use a random UUID) to pass requests to githooker. Here is an example `nginx` location:
```
        location /cd7f1d6b-bf5d-4327-a8b5-6e291a96567a {
            proxy_set_header X-Real-IP  $remote_addr;
            proxy_set_header Host $host;
            proxy_set_header X-Scheme $scheme;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_pass http://127.0.0.1:4040;
        }
```
3) Set up the required environment variables for your installation. At a minimum, you will need to set `GH_HMAC_KEY` with a secret value you will configure as part of your github hook setup (once again, I recommend a random UUID).
4) I recommend running githooker from the command line first while setting up the github hooks for your repositories. You will be able to see the full path of the hook command `githooker` will be attempting to run. Command paths are constructed using the `GH_CMD_ROOT` environment variable, the full repository name, and the ref path of the hook. For example, after pushing an update to the `githooker` project, the service would attempt to execute the following command:
```
\etc\githooker\smeans\githooker\refs\heads\main
```
On Linux, this can be a shell script, an executable, a symlink, etc. On Windows, executables must have a `.exe` extension, so you will need to set the `GH_CMD_EXTENSIONS` variable:
```
set GH_CMD_EXTENSIONS=.exe .cmd .ps1
```
The service will try each extension in turn, stopping after successfully executing a command.
5) After establishing connectivity and testing your hook setup, it is best to run `githooker` as a service. Put the executable you built in `/usr/local/bin`. Here is a sample Linux `systemd` service file:

```
[Unit]
Description=githooker github hook processor
Documentation=https://github.com/smeans/githooker
After=network.target

[Service]
Environment=GH_HMAC_KEY=(secret goes here)
Type=simple
ExecStart=/usr/local/bin/githooker
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Save this file in `/lib/systemd/system/githooker.service` and enable it:

```
systemctl enable githooker --now
```
