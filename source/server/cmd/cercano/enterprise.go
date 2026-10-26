package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode"

	"cercano/source/server/internal/enterprise"
	"cercano/source/server/pkg/proto"
	v1 "github.com/bryancostanich/Cercano/source/enterpriseapi/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

func newEnterpriseHost() (*enterprise.Host, error) {
	dir, err := enterprise.DefaultStateDir()
	if err != nil {
		return nil, err
	}
	return enterprise.NewHost(enterprise.HostOptions{Directory: dir, Manager: enterprise.Options{ClientVersion: version}})
}
func openEnterpriseBrowser(ctx context.Context, target string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("enterprise login currently supports macOS")
	}
	return exec.CommandContext(ctx, "open", target).Run()
}

// enterpriseNotices receives connection and auto-launch notices, which stay
// off stdout so --json output remains machine-readable.
var enterpriseNotices io.Writer = os.Stderr

// isEnterpriseCommand reports whether name is an enterprise subcommand.
func isEnterpriseCommand(name string) bool {
	switch name {
	case "login", "sync", "status", "policy", "skills", "logout", "standalone":
		return true
	}
	return false
}

// enterpriseFlags are the flags of every enterprise subcommand, shared with
// enterpriseDialsProfile so both read --address alike.
type enterpriseFlags struct {
	*flag.FlagSet
	asJSON                        *bool
	skillID, server, org, address *string
}

func newEnterpriseFlags(out io.Writer) enterpriseFlags {
	fs := flag.NewFlagSet("enterprise", flag.ContinueOnError)
	fs.SetOutput(out)
	return enterpriseFlags{
		FlagSet: fs,
		asJSON:  fs.Bool("json", false, "print machine-readable output"),
		skillID: fs.String("skill", "", "namespaced enterprise skill ID to read"),
		server:  fs.String("server", "", "trusted enterprise HTTPS origin"),
		org:     fs.String("organization", "", "organization ID from your administrator"),
		address: fs.String("address", "", "running local Cercano host (default: the active profile's host)"),
	}
}

// enterpriseDialsProfile reports whether `cercano enterprise` with args
// connects to (and may start) the active profile's agent host: a supported
// subcommand on macOS whose flags parse and name no --address.
func enterpriseDialsProfile(args []string) bool {
	if len(args) == 0 || !isEnterpriseCommand(args[0]) || runtime.GOOS != "darwin" {
		return false
	}
	flags := newEnterpriseFlags(io.Discard)
	return flags.Parse(args[1:]) == nil && flags.NArg() == 0 && *flags.address == ""
}

// Connection changes run in the host. This CLI never opens Keychain, rotates a
// refresh token, or treats a downloaded bundle as proof of host enforcement.
func enterpriseCommand(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(out, "Usage: cercano enterprise login --server https://enterprise.example --organization UUID | sync | status | policy | skills [--skill ID] | logout | standalone\nConnects to the active profile's Cercano host (CERCANO_PROFILE), starting it if needed. Use --address 127.0.0.1:PORT to reach a specific host instead. Add --json for machine-readable output. Logout keeps managed work blocked; standalone explicitly returns to personal settings.")
		return nil
	}
	if !isEnterpriseCommand(args[0]) {
		return errors.New("unknown enterprise command")
	}
	if runtime.GOOS != "darwin" {
		return errors.New("enterprise account management currently supports macOS")
	}
	flags := newEnterpriseFlags(out)
	asJSON, skillID, server, org, address := flags.asJSON, flags.skillID, flags.server, flags.org, flags.address
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected enterprise arguments")
	}
	if args[0] != "login" && (*server != "" || *org != "") {
		return errors.New("server and organization apply only to login")
	}
	if *skillID != "" && (args[0] != "skills" || !strings.HasPrefix(*skillID, "enterprise/")) {
		return errors.New("--skill requires the skills command and an enterprise/ ID from its catalog")
	}
	var conn grpc.ClientConnInterface
	if *address != "" {
		host, _, err := net.SplitHostPort(*address)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return errors.New("enterprise controls require a literal loopback host address")
		}
		cc, err := grpc.NewClient(*address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return err
		}
		defer cc.Close()
		conn = cc
	} else {
		agent, err := dialActiveProfile(ctx, enterpriseNotices)
		if err != nil {
			return err
		}
		defer agent.Close()
		// Count as the agent's client until done (released before the
		// client closes), so an agent this command auto-launched exits after.
		defer holdAgent(ctx, agent.SubscribeEvents)()
		conn = agent.Conn()
	}
	timeout := 90 * time.Second
	if args[0] == "login" {
		timeout = 6 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client := proto.NewEnterpriseClient(conn)
	empty := &proto.EnterpriseControlRequest{}
	var err error
	var result *proto.EnterpriseStatus
	var policyResult *proto.EnterprisePolicy
	var skillsResult *proto.ListSkillsResponse
	var skillResult *proto.GetSkillResponse
	switch args[0] {
	case "login":
		result, err = client.Login(ctx, &proto.EnterpriseLoginRequest{Server: *server, OrganizationId: *org})
	case "sync":
		result, err = client.Synchronize(ctx, empty)
	case "status":
		result, err = client.GetStatus(ctx, empty)
	case "policy":
		policyResult, err = client.GetPolicy(ctx, empty)
	case "skills":
		agent := proto.NewAgentClient(conn)
		if *skillID != "" {
			skillResult, err = agent.GetSkill(ctx, &proto.GetSkillRequest{Name: *skillID})
		} else {
			skillsResult, err = agent.ListSkills(ctx, &proto.ListSkillsRequest{})
		}
	case "logout":
		result, err = client.Logout(ctx, empty)
	case "standalone":
		result, err = client.UseStandalone(ctx, empty)
	}
	if status.Code(err) == codes.Unimplemented {
		return errors.New("running host does not support enterprise management; update and restart the host")
	}
	if status.Code(err) == codes.Unavailable {
		return errors.New("cannot reach the Cercano host; start 'cercano agent' and retry")
	}
	if err != nil {
		return err
	}
	if policyResult != nil {
		return writeEnterprisePolicy(out, policyResult.PolicyJson, *asJSON)
	}
	if skillsResult != nil {
		return writeEnterpriseSkills(out, skillsResult, *asJSON)
	}
	if skillResult != nil {
		return writeEnterpriseSkill(out, skillResult, *asJSON)
	}
	return writeEnterpriseStatus(out, result, *asJSON)
}

func writeEnterpriseStatus(out io.Writer, result *proto.EnterpriseStatus, asJSON bool) error {
	if !asJSON {
		_, err := io.WriteString(out, enterpriseStatusText(result))
		return err
	}
	encoded, err := (protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true, Indent: "  "}).Marshal(result)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(encoded))
	return err
}

// Quote externally supplied strings so names cannot inject terminal controls.
func enterpriseStatusText(s *proto.EnterpriseStatus) string {
	var out strings.Builder
	mode := "Personal settings"
	if s.Managed {
		mode = "Enterprise managed"
	}
	fmt.Fprintf(&out, "Mode: %s\n", mode)
	if !s.Connected {
		if s.Managed {
			out.WriteString("Not signed in. Managed work is blocked. Run 'cercano enterprise login' to reconnect, or 'cercano enterprise standalone' to return to personal settings.\n")
		}
		return out.String()
	}
	if s.MembershipKnown {
		fmt.Fprintf(&out, "Organization: %q (%q)\n", s.OrganizationName, s.OrganizationId)
		if s.TeamId == "" {
			out.WriteString("Team: None assigned (organization policy applies)\n")
		} else {
			fmt.Fprintf(&out, "Team: %q (%q)\n", s.TeamName, s.TeamId)
		}
	} else {
		fmt.Fprintf(&out, "Organization: %q\nTeam: Details unavailable until a supporting server supplies them\n", s.OrganizationId)
	}
	fmt.Fprintf(&out, "Host: %q\n", s.HostId)
	if s.Revision > 0 {
		fmt.Fprintf(&out, "Applied policy revision: %d\nAuthorization valid until: %q\n", s.Revision, s.ValidUntil)
	} else {
		out.WriteString("Policy: Awaiting verified synchronization\n")
	}
	switch {
	case s.Changing:
		out.WriteString("Status: Updating enterprise connection\n")
	case !s.Usable:
		out.WriteString("Status: Managed work is blocked. Run 'cercano enterprise sync'; if access was revoked, contact your administrator.\n")
	case s.Error != "":
		out.WriteString("Status: Using the last verified policy while synchronization recovers\n")
	default:
		out.WriteString("Status: Ready; the running host has applied the policy\n")
	}
	if s.Error != "" {
		fmt.Fprintf(&out, "Last synchronization error: %q\n", s.Error)
	}
	if s.Revision > 0 && (s.Error != "" || !s.Usable) {
		out.WriteString("Organization and team details describe the last applied policy.\n")
	}
	return out.String()
}

func writeEnterprisePolicy(out io.Writer, raw []byte, asJSON bool) error {
	p, err := v1.DecodePolicy(raw)
	if err != nil {
		return errors.New("running host returned an unsupported policy; update the CLI")
	}
	if asJSON {
		encoded, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(encoded))
		return err
	}
	var text strings.Builder
	fmt.Fprintf(&text, "Enterprise policy revision %d\nOrganization: %q\nAuthorization valid until: %s\n\nAllowed model routes:\n", p.Revision, p.Scope.OrganizationID, p.ExpiresAt.UTC().Format(time.RFC3339))
	if len(p.AllowedRoutes) == 0 {
		text.WriteString("  None. All managed model requests are blocked.\n")
	}
	for _, r := range p.AllowedRoutes {
		fmt.Fprintf(&text, "  %q: %q / %q at %q (%s)\n", r.ID, r.Provider, r.Model, r.Endpoint, r.Placement)
	}
	text.WriteString("\nTask defaults and fallback order:\n")
	if len(p.TaskDefaults) == 0 {
		text.WriteString("  None assigned.\n")
	}
	for _, d := range p.TaskDefaults {
		choice := "Locked by administrator"
		if d.AllowDeveloperOverride {
			choice = "Developer may choose another approved route"
		}
		fmt.Fprintf(&text, "  %q (%q, %q): %q\n    Fallback order: %q\n    %s\n", d.Task, d.Destination, d.Quality, d.RouteID, d.FallbackRouteIDs, choice)
	}
	text.WriteString("\nThese are the host's current verified settings. Each model attempt still checks current authorization.\n")
	_, err = io.WriteString(out, text.String())
	return err
}
func writeEnterpriseSkills(out io.Writer, all *proto.ListSkillsResponse, asJSON bool) error {
	selected := &proto.ListSkillsResponse{}
	for _, s := range all.Skills {
		if s.Source == "enterprise" {
			selected.Skills = append(selected.Skills, s)
		}
	}
	if asJSON {
		encoded, err := (protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true, Indent: "  "}).Marshal(selected)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(encoded))
		return err
	}
	var text strings.Builder
	if len(selected.Skills) == 0 {
		text.WriteString("No enterprise skills are assigned.\n")
	}
	for _, s := range selected.Skills {
		fmt.Fprintf(&text, "%q — version %q (enterprise)\n  ID: %q\n  %q\n", s.DisplayName, s.Version, s.Name, s.Description)
	}
	if len(selected.Skills) > 0 {
		text.WriteString("Read a skill with 'cercano enterprise skills --skill ID'.\n")
	}
	_, err := io.WriteString(out, text.String())
	return err
}
func writeEnterpriseSkill(out io.Writer, s *proto.GetSkillResponse, asJSON bool) error {
	if s.Source != "enterprise" {
		return errors.New("the requested skill is not an assigned enterprise skill")
	}
	if asJSON {
		encoded, err := (protojson.MarshalOptions{UseProtoNames: true, Indent: "  "}).Marshal(s)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(encoded))
		return err
	}
	// Keep Markdown readable while escaping terminal controls in administrator text.
	var safe strings.Builder
	for _, r := range s.Content {
		if unicode.IsPrint(r) || r == '\n' || r == '\t' {
			safe.WriteRune(r)
		} else {
			fmt.Fprintf(&safe, "\\u%04x", r)
		}
	}
	_, err := fmt.Fprintf(out, "%q — version %q (enterprise)\n\n%s\n", s.Name, s.Version, safe.String())
	return err
}
