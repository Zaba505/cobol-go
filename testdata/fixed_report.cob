000100*****************************************************************
000200* REPORT - a fuller fixed-format (reference format) program.
000300* Exercises comments attached across every division and at a
000400* data entry and a sentence, plus PERFORM VARYING, COMPUTE,
000500* IF/ELSE, an intrinsic FUNCTION, and an 88-level condition.
000600*****************************************************************
000700 IDENTIFICATION DIVISION.
000800 PROGRAM-ID. REPORT.
000900*--------------------------------------------------------------
001000* This banner attaches to the ENVIRONMENT DIVISION.
001100*--------------------------------------------------------------
001200 ENVIRONMENT DIVISION.
001300 CONFIGURATION SECTION.
001400 SOURCE-COMPUTER. IBM-370.
001500 OBJECT-COMPUTER. IBM-370.
001600*--------------------------------------------------------------
001700* This banner attaches to the DATA DIVISION.
001800*--------------------------------------------------------------
001900 DATA DIVISION.
002000 WORKING-STORAGE SECTION.
002100 01 WS-COUNTER PIC 9(2) USAGE COMP-3 VALUE ZERO.
002200 01 WS-TOTAL   PIC S9(5)V99 VALUE 0.
002300* A comment before a data entry attaches to that entry.
002400 01 WS-LABEL   PIC X(10) VALUE "total".
002500 01 WS-FLAG    PIC X VALUE "N".
002600     88 WS-DONE VALUE "Y".
002700*--------------------------------------------------------------
002800* This banner attaches to the PROCEDURE DIVISION.
002900*--------------------------------------------------------------
003000 PROCEDURE DIVISION.
003100 MAIN-PARAGRAPH.
003200*    Accumulate 1 through 5 into the running total.
003300     PERFORM VARYING WS-COUNTER FROM 1 BY 1 UNTIL WS-COUNTER > 5
003400         COMPUTE WS-TOTAL = WS-TOTAL + WS-COUNTER
003500     END-PERFORM.
003600*    Upper-case the label with an intrinsic function.
003700     MOVE FUNCTION UPPER-CASE(WS-LABEL) TO WS-LABEL.
003800     IF WS-TOTAL > 10 THEN
003900         DISPLAY WS-LABEL " = " WS-TOTAL
004000     ELSE
004100         DISPLAY "Total is ten or less."
004200     END-IF.
004300     STOP RUN.
