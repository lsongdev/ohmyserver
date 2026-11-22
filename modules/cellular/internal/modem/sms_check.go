package modem

import (
	"context"
	"errors"
	"strings"

	"github.com/lsongdev/ohmyserver/modules/cellular/internal/at"
)

// SMSCheck tests prerequisites. Passing does not guarantee carrier acceptance.
type SMSCheck struct {
	Ready          bool     `json:"ready"`
	SIM            string   `json:"sim"`
	CS             *int     `json:"cs_registration,omitempty"`
	LTE            *int     `json:"lte_registration,omitempty"`
	NR             *int     `json:"nr_registration,omitempty"`
	Service        *int     `json:"service,omitempty"`
	SMSCConfigured bool     `json:"smsc_configured"`
	IMSMode        *int     `json:"ims_mode,omitempty"`
	Issues         []string `json:"issues"`
}

type SMSFailure struct {
	Message string
	Cause   error
}

func (e *SMSFailure) Error() string { return e.Message }
func (e *SMSFailure) Unwrap() error { return e.Cause }

func registered(status *int) bool { return status != nil && (*status == 1 || *status == 5) }

func (d Device) CheckSMS(ctx context.Context) (SMSCheck, error) {
	check := SMSCheck{Issues: []string{}}
	var err error
	check.SIM, err = d.SIM(ctx)
	if err != nil {
		return check, err
	}
	if check.SIM != "READY" {
		check.Issues = append(check.Issues, "SIM is not ready: "+check.SIM)
	}
	for _, query := range []struct {
		command, prefix string
		target          **int
	}{
		{"AT+CREG?", "+CREG:", &check.CS},
		{"AT+CEREG?", "+CEREG:", &check.LTE},
		{"AT+C5GREG?", "+C5GREG:", &check.NR},
	} {
		*query.target, err = d.registration(ctx, query.command, query.prefix)
		if err != nil && !replyError(err) {
			return check, err
		}
	}
	if !registered(check.CS) && !registered(check.LTE) && !registered(check.NR) {
		check.Issues = append(check.Issues, "The modem is not registered on a mobile network.")
	}
	value, err := d.value(ctx, "AT+CGSMS?", "+CGSMS:")
	if err != nil {
		return check, err
	}
	check.Service = number([]string{value}, 0)
	if check.Service == nil || *check.Service < 0 || *check.Service > 3 {
		return check, InvalidReply("AT+CGSMS?")
	}
	if *check.Service == 1 && !registered(check.CS) {
		check.Issues = append(check.Issues, "SMS is set to circuit-switched only, but that network is not registered. Configure packet-domain preferred (AT+CGSMS=2).")
	}
	value, err = d.value(ctx, "AT+CSCA?", "+CSCA:")
	if err != nil {
		return check, err
	}
	values := fields("+CSCA: " + value)
	check.SMSCConfigured = len(values) > 0 && strings.TrimSpace(values[0]) != ""
	if !check.SMSCConfigured {
		check.Issues = append(check.Issues, "No SMS service centre is configured. Obtain the correct number from the carrier.")
	}
	// QCFG reports configuration and capability, not IMS registration.
	value, err = d.value(ctx, `AT+QCFG="ims"`, "+QCFG:")
	if err != nil && !replyError(err) {
		return check, err
	}
	if err == nil {
		check.IMSMode = number(fields("+QCFG: "+value), 1)
	}
	check.Ready = len(check.Issues) == 0
	return check, nil
}

func smsFailure(err error) error {
	var rejected at.Rejection
	if errors.As(err, &rejected) && strings.TrimSpace(payload(string(rejected))) == "350" &&
		strings.HasPrefix(string(rejected), "+CMS ERROR:") {
		return &SMSFailure{
			Message: "SMS submission failed (+CMS ERROR: 350). Check the carrier SMS service and IMS configuration; mobile network registration alone does not confirm SMS availability.",
			Cause:   err,
		}
	}
	return err
}

func (m *Monitor) CheckSMS(ctx context.Context) (SMSCheck, error) {
	var check SMSCheck
	err := m.execute(ctx, func(ctx context.Context, device *Device) (err error) {
		check, err = device.CheckSMS(ctx)
		return err
	})
	return check, err
}
