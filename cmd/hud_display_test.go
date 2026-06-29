package cmd

import "testing"

func TestAnimName(t *testing.T) {
	cases := []struct {
		name string
		id   AnimationID
		want string
	}{
		{"eye idle", Anim_EyeIdle, "Idle"},
		{"eye blink", Anim_EyeBlink, "Blink"},
		{"eye happy", Anim_EyeHappy, "Happy"},
		{"eye excited", Anim_EyeExcited, "Excited"},
		{"eye excited left", Anim_EyeExcitedLeft, "Excited"},
		{"eye excited right", Anim_EyeExcitedRight, "Excited"},
		{"eye stare", Anim_EyeStare, "Stare"},
		{"mouth stare", Anim_MouthStare, "Stare"},
		{"mouth stare left", Anim_MouthStareLeft, "Stare"},
		{"mouth stare right", Anim_MouthStareRight, "Stare"},
		{"eye flushed", Anim_EyeFlushed, "Flushed"},
		{"mouth flushed", Anim_MouthFlushed, "Flushed"},
		{"mouth flushed left", Anim_MouthFlushedLeft, "Flushed"},
		{"mouth flushed right", Anim_MouthFlushedRight, "Flushed"},
		{"mouth idle", Anim_MouthIdle, "Idle"},
		{"mouth idle left", Anim_MouthIdleLeft, "Idle"},
		{"mouth idle right", Anim_MouthIdleRight, "Idle"},
		{"mouth yap1", Anim_MouthYap1, "Yap1"},
		{"mouth yap1 left", Anim_MouthYap1Left, "Yap1"},
		{"mouth yap1 right", Anim_MouthYap1Right, "Yap1"},
		{"mouth yap2", Anim_MouthYap2, "Yap2"},
		{"mouth yap2 left", Anim_MouthYap2Left, "Yap2"},
		{"mouth yap2 right", Anim_MouthYap2Right, "Yap2"},
		{"mouth yap3", Anim_MouthYap3, "Yap3"},
		{"mouth yap3 left", Anim_MouthYap3Left, "Yap3"},
		{"mouth yap3 right", Anim_MouthYap3Right, "Yap3"},
		{"unknown", AnimationID(0xFF), "?"},
	}
	for _, c := range cases {
		if got := animName(c.id); got != c.want {
			t.Errorf("animName(%s/%v) = %q, want %q", c.name, c.id, got, c.want)
		}
	}
}

func TestExpressionSet(t *testing.T) {
	cases := []struct {
		name string
		eye  AnimationID
		want string
	}{
		{"happy", Anim_EyeHappy, "Happy"},
		{"excited", Anim_EyeExcited, "Excited"},
		{"excited left", Anim_EyeExcitedLeft, "Excited"},
		{"excited right", Anim_EyeExcitedRight, "Excited"},
		{"stare", Anim_EyeStare, "Stare"},
		{"flushed", Anim_EyeFlushed, "Flushed"},
		{"default idle", Anim_EyeIdle, "None"},
		{"default blink", Anim_EyeBlink, "None"},
	}
	for _, c := range cases {
		if got := expressionSet(c.eye); got != c.want {
			t.Errorf("expressionSet(%s/%v) = %q, want %q", c.name, c.eye, got, c.want)
		}
	}
}

func TestFormatVolts(t *testing.T) {
	cases := []struct {
		deci uint8
		want string
	}{
		{74, "7.4V"},
		{84, "8.4V"},
		{66, "6.6V"},
		{100, "10.0V"},
		{7, "0.7V"},
		{0, "0.0V"},
	}
	for _, c := range cases {
		if got := formatVolts(c.deci); got != c.want {
			t.Errorf("formatVolts(%d) = %q, want %q", c.deci, got, c.want)
		}
	}
}
