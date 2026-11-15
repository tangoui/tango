package text

import "testing"

func TestShapeIntoReusesNumericLayout(t *testing.T) {
 var l Layout
 a:=ShapeInto(&l,"12.34",Style{Size:11},Options{NoWrap:true})
 if a!=&l{t.Fatal("numeric ShapeInto did not return destination")}
 glyphs:=&l.Glyphs[0]
 a=ShapeInto(&l,"87.01",Style{Size:11},Options{NoWrap:true})
 if a!=&l||&l.Glyphs[0]!=glyphs{t.Fatal("numeric layout buffers were replaced")}
}
