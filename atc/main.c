#include "at_toks.h"
#include "std.h"
#include "std_io.h"


/*

fairly unidiomatic code! because we want to have a most compact C code base to
later transliterate smoothly into our initially very limited language iteration:

- no proper error handling, immediate `panic`s upon detecting a problem
- no 3rd-party imports / deps whatsoever
- no stdlib imports for *core* processing (just for basic program setup & I/O)
  (hence manual implementations like uintToStr, uintParse, strEql etc)
- use of macros limited to eventual-WIP-lang meta-programming / generic facilities
- all would-be `malloc`s replaced by global fixed-size backing buffer allocation
- naming / casing conventions follow WIP target lang rather than C customs

*/


int main(int argc, String argv[]) {
    if (argc < 2)
        panic("expected usage: atc <src_file_path>");

    Str input_src_file_bytes = readFile(argv[1]);

    Tokens toks = tokenize(input_src_file_bytes, false);
    toksCheckBrackets(toks);
}
