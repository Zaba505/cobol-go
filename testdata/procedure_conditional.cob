IDENTIFICATION DIVISION.
PROGRAM-ID. CONDDEMO.
PROCEDURE DIVISION.
    IF WS-A > WS-B THEN
        MOVE 1 TO WS-C
    ELSE
        MOVE 2 TO WS-C
    END-IF.
    IF WS-A = WS-B AND WS-C < 10 OR NOT WS-D = 0
        DISPLAY "yes"
    END-IF.
    IF WS-A IS NUMERIC
        CONTINUE
    END-IF.
    IF WS-A IS NOT POSITIVE
        ADD 1 TO WS-A
    END-IF.
    IF WS-FLAG
        DISPLAY "flag set"
    END-IF.
    IF WS-A GREATER THAN WS-B
        MOVE WS-A TO WS-B
    END-IF.
    IF WS-A = 1
        IF WS-B = 2
            DISPLAY "both"
        END-IF
    END-IF.
    IF WS-A = 0 DISPLAY "zero".
    STOP RUN.
