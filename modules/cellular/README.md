# Cellular

The Cellular panel reads Quectel RM520N modem status directly over its AT
serial interface. It displays the modem, SIM, LTE and 5G registration, and
serving-cell signal. It queries only status and leaves data session management
to the host.

On Linux, the poller locates USB ID `2c7c:0801`, interface `02`, under sysfs.
Set `cellular_port` in OhMyServer's config to select an explicit AT device.
The serial connection uses 115200 baud. Each process caches the latest poll
for HTTP clients and reopens the port after transport failures.

## Run independently

Run one owner of the AT port at a time. Stop the integrated monitor's
OhMyServer process before running the standalone server on the same modem.
The standalone builds and serves the same panel and API.

```sh
make deps
go run ./modules/cellular/cmd/cellular -listen 127.0.0.1:10181
```

Use `-port /dev/ttyUSB2` to override detection. Set `CELLULAR_PASSWORD` to
require panel sign-in. The status endpoint is `GET /m/cellular/api/status`.

## SMS

SMS is available when host sign-in is configured. In OhMyServer, use the
panel password. In the standalone server, set `CELLULAR_PASSWORD`.

The SMS page reads both supported physical stores, SIM (`SM`) and modem
(`ME`), and labels each message's sources. The original CPMS read store is
restored after loading. Opening the page can mark unread records as read.
A failure in one store is reported alongside messages from the other store.

Long SMS messages are assembled by their protocol headers, sender, encoding
and timestamp window. Duplicate PDUs retain all storage locations. Missing
parts remain visible with their sequence numbers, and conflicting parts are
shown separately. The list is sorted by message time.

The modem sends SMS in PDU mode with GSM-7 and Unicode support, including up
to six parts. Confirmation means the modem accepted the submission. Delivery
depends on the carrier.

Before each submission the driver checks SIM readiness, CS/LTE/5G
registration, SMS domain selection and the presence of an SMS service
centre. The SMS page displays configuration conflicts. The driver reports
CMS 350 with guidance to check the carrier service and IMS, retaining the
original modem error. It never automatically retries a submitted message.

On the Photonicat's RM520N-GL with China Unicom 5G SA, the tested working
configuration is packet-domain preferred (`AT+CGSMS=2`) and explicitly
enabled IMS (`AT+QCFG="ims",1`), followed by a modem restart.
These settings are configured explicitly through the AT page. Changing them
is separate from sending a message. The second value in the IMS configuration
reply describes capability and does not establish IMS registration.
