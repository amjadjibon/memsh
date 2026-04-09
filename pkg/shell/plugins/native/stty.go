package native

import (
	"context"
	"fmt"

	"github.com/amjadjibon/memsh/pkg/shell/plugins"
	"mvdan.cc/sh/v3/interp"
)

type SttyPlugin struct{}

func (SttyPlugin) Name() string                                 { return "stty" }
func (SttyPlugin) Description() string                          { return "terminal settings stub" }
func (SttyPlugin) Usage() string                                { return "stty [size | cols | rows | -a | setting...]" }
func (SttyPlugin) Run(ctx context.Context, args []string) error { return runStty(ctx, args) }

const sttyAll = `speed 38400 baud; rows 24; columns 80; line = 0;
intr = ^C; quit = ^\; erase = ^?; kill = ^U; eof = ^D; eol = <undef>;
eol2 = <undef>; swtch = <undef>; start = ^Q; stop = ^S; susp = ^Z; rprnt = ^R;
werase = ^W; lnext = ^V; discard = ^O; min = 1; time = 0;
-parenb -parodd -cmspar cs8 -hupcl -cstopb cread -clocal -crtscts
-ignbrk -brkint -ignpar -parmrk -inpck -istrip -inlcr -igncr icrnl ixon -ixoff
-iuclc -ixany -imaxbel -iutf8
opost -olcuc -ocrnl onlcr -onocr -onlret -ofill -ofdel nl0 cr0 tab0 bs0 vt0 ff0
isig icanon iexten echo echoe echok -echonl -noflsh -xcase -tostop -echoprt
echoctl echoke -flusho -extproc
`

func runStty(ctx context.Context, args []string) error {
	hc := interp.HandlerCtx(ctx)
	_ = plugins.ShellCtx(ctx)

	if len(args) < 2 {
		// bare stty → print speed and settings (minimal)
		fmt.Fprintf(hc.Stdout, "speed 38400 baud; rows %d; columns %d;\n", termLines, termCols)
		return nil
	}

	switch args[1] {
	case "size":
		fmt.Fprintf(hc.Stdout, "%d %d\n", termLines, termCols)
	case "cols", "columns":
		fmt.Fprintln(hc.Stdout, termCols)
	case "rows":
		fmt.Fprintln(hc.Stdout, termLines)
	case "-a", "--all":
		fmt.Fprintf(hc.Stdout, sttyAll)
	case "-g", "--save":
		// print a restorable settings string (fake)
		fmt.Fprintf(hc.Stdout, "1:0:bf:8a3b:3:1c:7f:15:4:0:1:0:11:13:1a:0:12:f:17:16:0:0:0:0:0:0:0:0:0:0:0:0:0:0:0:0\n")
	default:
		// all other settings (echo, raw, -echo, speed, etc.) → no-op
	}
	return nil
}
