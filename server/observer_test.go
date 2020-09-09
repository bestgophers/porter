package server

func ExampleObserver() {
	subject := NewSubject()
	r1 := NewReader("reader1")
	r2 := NewReader("reader2")
	r3 := NewReader("reader3")
	subject.Attach(r1)
	subject.Attach(r2)
	subject.Attach(r3)

	subject.UpdateContext("observer mode")
	// Output:
	// reader1 receive observer mode
	// reader2 receive observer mode
	// reader3 receive observer mode
}
