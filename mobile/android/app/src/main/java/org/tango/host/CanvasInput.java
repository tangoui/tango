package org.tango.host;

import android.text.Editable;
import android.text.TextUtils;
import android.view.KeyEvent;
import android.view.inputmethod.BaseInputConnection;
import android.view.inputmethod.ExtractedText;
import android.view.inputmethod.ExtractedTextRequest;

/**
 * The input connection of a {@link CanvasView} while a Tango text field has
 * focus (canvas_ime.go, docs/specs/mobile-canvas.md §2.3).
 *
 * <p>Edits (composing, committing, deleting, picking a region, moving the
 * selection, Delete and Enter) go to Go synchronously, one JNI call each
 * ({@link CanvasView#ime}); Go turns them into input events and sends back
 * the new document, which the view keeps as its mirror. Queries
 * (text around the cursor, the selection) are answered from that mirror
 * without crossing JNI.
 *
 * <p>Positions are UTF-16 offsets in the mirror plus its base (the field
 * position of the mirror's first character). A connection the view has
 * replaced (restartInput) ignores every call.
 */
final class CanvasInput extends BaseInputConnection {
    // nativeIME operations (canvas_ime_android.go).
    static final int OP_COMPOSE = 1, OP_COMMIT = 2, OP_FINISH = 3, OP_DELETE = 4, OP_DELETE_CP = 5,
            OP_REGION = 6, OP_SELECT = 7, OP_BACKSPACE = 8, OP_ENTER = 9, OP_RESET = 10;

    private final CanvasView view;
    private boolean closed;

    CanvasInput(CanvasView view) {
        super(view, true);
        this.view = view;
    }

    private boolean live() {
        return !closed && view.liveInput(this);
    }

    private boolean op(int op, CharSequence text, int a, int b) {
        if (!live()) return false;
        view.ime(op, text == null ? null : text.toString(), a, b);
        return true;
    }

    @Override
    public Editable getEditable() {
        return view.mirror;
    }

    // ---- edits (Go) ----

    @Override
    public boolean setComposingText(CharSequence text, int newCursorPosition) {
        return op(OP_COMPOSE, text, newCursorPosition, 0);
    }

    @Override
    public boolean commitText(CharSequence text, int newCursorPosition) {
        return op(OP_COMMIT, text, newCursorPosition, 0);
    }

    @Override
    public boolean finishComposingText() {
        return op(OP_FINISH, null, 0, 0);
    }

    @Override
    public boolean deleteSurroundingText(int beforeLength, int afterLength) {
        return op(OP_DELETE, null, beforeLength, afterLength);
    }

    @Override
    public boolean deleteSurroundingTextInCodePoints(int beforeLength, int afterLength) {
        return op(OP_DELETE_CP, null, beforeLength, afterLength);
    }

    @Override
    public boolean setComposingRegion(int start, int end) {
        return op(OP_REGION, null, start, end);
    }

    @Override
    public boolean setSelection(int start, int end) {
        return op(OP_SELECT, null, start, end);
    }

    @Override
    public boolean performEditorAction(int editorAction) {
        return op(OP_ENTER, null, 0, 0);
    }

    /**
     * Delete and Enter edit the document (a pre-edit in progress first), so
     * they go through Go synchronously; other keys take the view's key path
     * (CanvasView.onKeyDown).
     */
    @Override
    public boolean sendKeyEvent(KeyEvent e) {
        if (!live()) return false;
        int k = e.getKeyCode();
        if (k == KeyEvent.KEYCODE_DEL || k == KeyEvent.KEYCODE_ENTER || k == KeyEvent.KEYCODE_NUMPAD_ENTER) {
            if (e.getAction() == KeyEvent.ACTION_DOWN) {
                view.ime(k == KeyEvent.KEYCODE_DEL ? OP_BACKSPACE : OP_ENTER, null, 0, 0);
            }
            return true;
        }
        return super.sendKeyEvent(e);
    }

    @Override
    public boolean beginBatchEdit() {
        if (!live()) return false;
        view.beginBatch();
        return true;
    }

    @Override
    public boolean endBatchEdit() {
        if (!live()) return false;
        return view.endBatch();
    }

    @Override
    public void closeConnection() {
        if (live()) {
            closed = true;
            view.ime(OP_RESET, null, 0, 0);
            view.endAllBatches();
        }
        closed = true;
        super.closeConnection();
    }

    // ---- queries (mirror) ----

    @Override
    public CharSequence getTextBeforeCursor(int n, int flags) {
        if (!live() || n < 0) return null;
        int s = view.mirrorSelStart();
        return TextUtils.substring(view.mirror, Math.max(0, s - n), s);
    }

    @Override
    public CharSequence getTextAfterCursor(int n, int flags) {
        if (!live() || n < 0) return null;
        int e = view.mirrorSelEnd();
        return TextUtils.substring(view.mirror, e, (int) Math.min((long) e + n, view.mirror.length()));
    }

    @Override
    public CharSequence getSelectedText(int flags) {
        if (!live()) return null;
        int s = view.mirrorSelStart(), e = view.mirrorSelEnd();
        return s == e ? null : TextUtils.substring(view.mirror, s, e);
    }

    @Override
    public int getCursorCapsMode(int reqModes) {
        if (!live()) return 0;
        return TextUtils.getCapsMode(view.mirror, view.mirrorSelStart(), reqModes);
    }

    /** No extracted text: the canvas turns the extract editor off (IME_FLAG_NO_EXTRACT_UI). */
    @Override
    public ExtractedText getExtractedText(ExtractedTextRequest request, int flags) {
        return null;
    }

    @Override
    public boolean requestCursorUpdates(int cursorUpdateMode) {
        if (!live()) return false;
        view.cursorUpdates(cursorUpdateMode);
        return true;
    }

    @Override
    public boolean requestCursorUpdates(int cursorUpdateMode, int cursorUpdateFilter) {
        return requestCursorUpdates(cursorUpdateMode | cursorUpdateFilter);
    }
}
