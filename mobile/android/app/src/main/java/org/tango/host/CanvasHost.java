package org.tango.host;

/**
 * What Go calls on every canvas, whichever kind (canvas_android.go looks
 * these methods up on this interface): a {@link CanvasPage} (a page of its
 * own, uihost.Open) or a {@link CanvasOverlay} (a canvas inside a WebView
 * page, uihost.Attach). Main thread.
 */
interface CanvasHost {
    /** Text input on or off, the EditorInfo inputType and imeOptions and the caret in device pixels (CanvasView.textInput). */
    void textInput(boolean on, int inputType, int imeOptions, float x, float y, float w, float h, boolean restart);

    /** The input method's document (CanvasView.mirror). */
    void mirror(String text, int base, int selStart, int selEnd, int compStart, int compEnd);

    /** A content change of a virtual view (CanvasA11y.C_* bits; 0 undefined). */
    void a11yEvent(int virtualId, int changes);

    /** The virtual view with input focus (CanvasA11y.NO_ID: none). */
    void a11yFocus(int virtualId);

    /** Text to speak once that no live region carries (ui.Context.Announce). */
    void a11yAnnounce(String text, int live);

    /** Debug self-test: performs an action on a virtual view through the provider. */
    void a11ySelfTest(int virtualId, int action);

    /** Sets the mouse pointer shape (a PointerIcon type). */
    void setCursor(int type);

    /**
     * Whether the canvas takes the system Back action (the UI has an
     * overlay, a drawer, a dialog to close): while on and the canvas is the
     * top-most visible one asking, Back arrives as Escape instead of
     * leaving the page (TangoActivity.back).
     */
    void setBackHandling(boolean on);
}
