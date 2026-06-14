000100*****************************************************************
000200* HELLO - a minimal fixed-format (reference format) program.
000300* Exercises the ignored sequence area (columns 1-6), the
000400* column-7 comment indicator, and Area A division/paragraph
000500* headers. The banner above attaches to the program.
000600*****************************************************************
000700 IDENTIFICATION DIVISION.
000800 PROGRAM-ID. HELLO.
000900/----------------------------------------------------------------
001000* A column-7 slash forces a page eject; it is still a comment.
001100* These two lines attach to the PROCEDURE DIVISION below.
001200 PROCEDURE DIVISION.
001300 MAIN-PARAGRAPH.
001400*    Greet the world, then stop the run unit.
001500     DISPLAY "Hello, fixed-format world!".
001600     STOP RUN.
