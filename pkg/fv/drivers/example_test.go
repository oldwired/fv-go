package drivers_test

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
)

// Producers push events with Put; the main loop pulls them with Get.
// This is the queue Program.Run consumes from.
func ExampleQueue() {
	q := drivers.NewQueue()
	q.Put(drivers.Event{What: consts.EvCommand, Command: consts.CmOK})
	q.Put(drivers.Event{What: consts.EvKeyDown, KeyCode: consts.KbEnter})

	for {
		ev, ok := q.Get()
		if !ok {
			break
		}
		fmt.Printf("what=%#x command=%d\n", ev.What, ev.Command)
	}
	// Output:
	// what=0x100 command=10
	// what=0x10 command=0
}
