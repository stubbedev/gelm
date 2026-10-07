package headlesstest

import (
	"os"
	"testing"
	"time"
)

// The single-instance gate (#79) under a real compositor: two
// processes claim the same AppID, the loser forwards its --open path
// and argv to the winner and exits, and the winner's shell trace shows
// the forwarded invocation. Everything rides on the same
// compositor/wire/loop path a desktop launch would.
func TestSingleInstanceForwardsToPrimary(t *testing.T) {
	if os.Getenv("GELM_HEADLESS") == "" {
		t.Skip("GELM_HEADLESS is not set; compositor input tests run under just check-headless")
	}
	bin, err := BuildClient(testEnv.Dir, "./cmd/gelm-messages", "gelm-messages")
	if err != nil {
		t.Fatal(err)
	}

	primary, err := testEnv.StartClient(bin, "instance-primary", "shell,frame")
	if err != nil {
		t.Fatal(err)
	}
	defer primary.Stop()
	w, err := primary.Watch()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Wait("shell", "instance dev.stubbe.gelm.messages: primary", traceTimeout); err != nil {
		t.Fatalf("primary never claimed the AppID: %v", err)
	}

	secondary, err := testEnv.StartClient(bin, "instance-secondary", "shell", "--open", "/tmp/gelm-opened.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer secondary.Stop()
	select {
	case <-secondary.Exited():
	case <-time.After(traceTimeout):
		t.Fatal("the secondary never exited after forwarding")
	}
	if err := secondary.Wait(); err != nil {
		t.Fatalf("secondary exit: %v", err)
	}
	if _, err := w.Wait("shell", "forwarded invocation (0 args, 1 open)", traceTimeout); err != nil {
		t.Fatalf("the primary never received the forward: %v", err)
	}
	select {
	case <-primary.Exited():
		t.Fatal("the primary died on the forwarded invocation")
	default:
	}
}
