# LMTPSympa

LMTPSympa is an LMTP/SMTP frontend for [Sympa](https://sympa.community)
mailing list management software (MLM).

## Install

1.  Install Go.  Go v1.17.x or later is required.

2.  Build LMTPSympa.  Run:
    ``` bash
    $ go install github.com/ikedas/lmtpsympa@latest
    ```
    and put the executable file `lmtpsympa` into anywhere you prefer.

3.  Create the configuration file `lmtpsympa.yml`.
    See also
    [`lmtpsympa.yml.sample`](https://github.com/ikedas/lmtpsympa/blob/main/lmtpsympa.yml.sample)
    in the repository.

Now you may run `lmtpsympa lmtpsympa.yml` to launch LMTPSympa.
LMTPSympa safely terminates on receipt of TERM or INT signal.

## Deploy

The following is an example of deployment with Systemd and Postfix.

### Configure LMTPSympa

Create `lmtpsympa.yml`:
``` yaml
%YAML 1.1
---
domain: mail.example.org
# Add the other virtual domains to "domains" if any
home: /var/lib/sympa/list_data
queue: /var/spool/sympa/msg
queuebounce: /var/spool/sympa/bounce
service:
  addr: :8024
  #addr: 127.0.0.1:8024                     # IPv4 loopback
  #addr: [::1]:8024                         # IPv6 loopback
  #addr: /var/run/sympa/lmtpsympa.socket    # UNIX domain socket
  mode: 660
```

### Register the service

Create `lmtpsympa.service`:
``` code
[Unit]
Description=LMTPSympa - LMTP/SMTP frontend for Sympa
After=syslog.target network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/lmtpsympa /usr/local/etc/lmtpsympa.yml
User=sympa
Group=postfix
KillMode=process
Restart=always

[Install]
WantedBy=multi-user.target
```
put it into appropriate place, `/etc/systemd/system`, `/usr/lib/systemd/system`
or somewhere, and run `systemctl daemon-reload`.

Then you may run `systemctl start lmtpsympa.service` to start the service.

*   If you configured LMTPSympa to use UNIX domain socket, make sure that
    the socket path is created and its permission allows Postfix to access to.
    ``` bash
    $ ls -l /var/run/sympa/lmtpsympa.socket
    srw-rw---- 1 sympa postfix 0 Mar 12 09:44 /var/run/sympa/lmtpsympa.socket
    ```
*   If you configured to use TCP socket, make sure that appropriate port is
    listened.

### Integrate to the MTA

Edit `main.cf` and the other configuration files of Postfix:

*   The existing configuration for Sympa should be disabled.

*   Add the domain(s) dedicated for mailing list service to
    the transport(5) table, for example:
    ``` code
    mail.example.org lmtp:inet:127.0.0.1:8024
    ...
    ```
    ``` code
    mail.example.org lmtp:inet:[::1]:8024
    ...
    ```
    ``` code
    mail.example.org lmtp:unix:/var/run/sympa/lmtpsympa.socket
    ...
    ```
*   Create `/etc/postfix/lmtpsympa_domains`:
    ``` code
    mail.example.org reject_unverified_recipient
    ...
    ```
    Don't forget to run `postmap /etc/postfix/lmtpsympa_domains`.

*   Add the domain(s) dedicated for mailing list service to
    `relay_domains`, and enable recipient varification for them, for example:
    ``` code
    relay_domains = $mydestination, hash:/etc/postfix/lmtpsympa_domains
    smtpd_recipient_restrictions = (existing settings...),
      check_recipient_access hash:/etc/postfix/lmtpsympa_domains
    unverified_recipient_reject_code = 550
    ```
    Note that the default value of `relay_domains` is the value of
    `mydestination`. It should therefore normally be included.

### Mofification on Sympa

*   If all deliveries to mailing list service are made by LMTPSympa,
    you may disable update of mail aliases.
    In `sympa.conf` and/or `robot.conf`, set:
    ``` code
    aliases_program none
    ```
*   For the same reason, setuid programs `queue` and `bouncequeue` should
    also be no longer needed.

---

## License

This software is licensed under the terms of MIT License.
See [LICENSE](LICENSE) file for more information.
