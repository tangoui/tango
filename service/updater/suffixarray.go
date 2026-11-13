package updater

// suffixArray returns the suffix array of b: the start offsets of b's
// suffixes in lexicographic order. It uses induced sorting (SA-IS, after
// Nong, Zhang and Chan, "Two Efficient Algorithms for Linear Time Suffix
// Array Construction", 2009), which runs in linear time, so building the
// index of a large executable stays fast even when it is very repetitive.
func suffixArray(b []byte) []int32 {
	n := len(b)
	sa := make([]int32, n)
	if n == 0 {
		return sa
	}
	s := make([]int32, n)
	for i, c := range b {
		s[i] = int32(c)
	}
	sais(s, sa, 256)
	return sa
}

// sais fills sa (len(s) entries) with the suffix array of s, whose values
// are in [0, k). The string is treated as if followed by a unique sentinel
// smaller than every value.
func sais(s, sa []int32, k int) {
	n := len(s)
	switch n {
	case 0:
		return
	case 1:
		sa[0] = 0
		return
	}

	// Classify every position: S-type (smaller than its successor
	// suffix) or L-type. The last position is L, as the sentinel follows.
	stype := make([]bool, n)
	for i := n - 2; i >= 0; i-- {
		stype[i] = s[i] < s[i+1] || s[i] == s[i+1] && stype[i+1]
	}
	lms := func(i int32) bool { return i > 0 && stype[i] && !stype[i-1] }

	counts := make([]int32, k)
	for _, c := range s {
		counts[c]++
	}
	bkt := make([]int32, k)
	heads := func() {
		var sum int32
		for c, n := range counts {
			bkt[c] = sum
			sum += n
		}
	}
	tails := func() {
		var sum int32
		for c, n := range counts {
			sum += n
			bkt[c] = sum
		}
	}
	induce := func() {
		// L-type suffixes, left to right, starting with the one before
		// the sentinel.
		heads()
		sa[bkt[s[n-1]]] = int32(n - 1)
		bkt[s[n-1]]++
		for i := 0; i < n; i++ {
			if j := sa[i] - 1; j >= 0 && !stype[j] {
				sa[bkt[s[j]]] = j
				bkt[s[j]]++
			}
		}
		// S-type suffixes, right to left.
		tails()
		for i := n - 1; i >= 0; i-- {
			if j := sa[i] - 1; j >= 0 && stype[j] {
				bkt[s[j]]--
				sa[bkt[s[j]]] = j
			}
		}
	}

	// Step 1: sort the LMS substrings by inducing from LMS positions
	// dropped at the ends of their buckets.
	for i := range sa {
		sa[i] = -1
	}
	tails()
	for i := n - 1; i > 0; i-- {
		if lms(int32(i)) {
			bkt[s[i]]--
			sa[bkt[s[i]]] = int32(i)
		}
	}
	induce()

	// Step 2: name the sorted LMS substrings; equal substrings share a
	// name. LMS positions are at least two apart, so position/2 indexes
	// a free slot in the upper half of sa.
	n1 := 0
	for i := 0; i < n; i++ {
		if lms(sa[i]) {
			sa[n1] = sa[i]
			n1++
		}
	}
	for i := n1; i < n; i++ {
		sa[i] = -1
	}
	name := int32(0)
	prev := int32(-1)
	for i := 0; i < n1; i++ {
		pos := sa[i]
		diff := prev < 0
		for d := int32(0); !diff; d++ {
			a, b := pos+d, prev+d
			if int(a) == n || int(b) == n || s[a] != s[b] || stype[a] != stype[b] {
				diff = true
			} else if d > 0 && (lms(a) || lms(b)) {
				break
			}
		}
		if diff {
			name++
			prev = pos
		}
		sa[n1+int(pos)/2] = name - 1
	}
	j := n - 1
	for i := n - 1; i >= n1; i-- {
		if sa[i] >= 0 {
			sa[j] = sa[i]
			j--
		}
	}

	// Step 3: sort the LMS suffixes, recursing on the reduced string when
	// names repeat.
	s1 := sa[n-n1:]
	sa1 := sa[:n1]
	if int(name) < n1 {
		sais(s1, sa1, int(name))
	} else {
		for i, c := range s1 {
			sa1[c] = int32(i)
		}
	}

	// Step 4: induce the full order from the sorted LMS suffixes.
	j = 0
	for i := 1; i < n; i++ {
		if lms(int32(i)) {
			s1[j] = int32(i)
			j++
		}
	}
	for i := range sa1 {
		sa1[i] = s1[sa1[i]]
	}
	for i := n1; i < n; i++ {
		sa[i] = -1
	}
	tails()
	for i := n1 - 1; i >= 0; i-- {
		p := sa[i]
		sa[i] = -1
		bkt[s[p]]--
		sa[bkt[s[p]]] = p
	}
	induce()
}
