# Voltix edge controller — systemd USER unit (no root, no host service).
# Template rendered by deploy/systemd/install.sh; @...@ placeholders are
# substituted with the install paths. Do not install this file directly.
# voltix-owner: @OWNER_TAG@
[Unit]
Description=Voltix edge controller (user demo instance)
After=network-online.target

[Service]
Type=simple
ExecStart=@BIN_PATH@ -config @CONFIG_PATH@
EnvironmentFile=@ENV_PATH@
# User-manager StateDirectory ignores StateDirectoryMode (creates 0777
# under systemd 259); chmod explicitly before start instead.
StateDirectory=@STATE_NAME@
ExecStartPre=/usr/bin/chmod 0700 %S/@STATE_NAME@
Restart=on-failure
RestartSec=2
MemoryMax=128M
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
StandardOutput=journal
StandardError=journal
SyslogIdentifier=voltix

[Install]
WantedBy=default.target
