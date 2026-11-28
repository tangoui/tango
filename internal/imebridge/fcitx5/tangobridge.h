#pragma once

#include "pipe.h"
#include <fcitx-utils/eventdispatcher.h>
#include <fcitx/action.h>
#include <fcitx/inputcontext.h>
#include <fcitx/instance.h>
#include <fcitx/inputmethodengine.h>
#include <fcitx/menu.h>
#include <unordered_map>

namespace tango::ime {

class TangoBridge final : public fcitx::InputMethodEngineV2 {
public:
    explicit TangoBridge(fcitx::Instance* instance);
    void activate(const fcitx::InputMethodEntry&, fcitx::InputContextEvent&) override;
    void deactivate(const fcitx::InputMethodEntry&, fcitx::InputContextEvent&) override;
    void reset(const fcitx::InputMethodEntry&, fcitx::InputContextEvent&) override;
    void keyEvent(const fcitx::InputMethodEntry&, fcitx::KeyEvent&) override;
    std::string subModeLabelImpl(const fcitx::InputMethodEntry&, fcitx::InputContext&) override;
    std::string subModeIconImpl(const fcitx::InputMethodEntry&, fcitx::InputContext&) override;

private:
    class Word;
    struct Context {
        fcitx::TrackableObjectReference<fcitx::InputContext> view;
        uint64_t session = 0;
        uint64_t opened = 0;
    };
    struct MenuAction {
        fcitx::SimpleAction action;
        fcitx::ScopedConnection handler;
    };

    Context& context(fcitx::InputContext*);
    bool open(Context&);
    void receive(uint64_t through = UINT64_MAX);
    void apply(Frame frame);
    void refreshMenu(const std::vector<MenuItem>& items);
    void clearPanel(fcitx::InputContext& context);
    void selectCandidate(fcitx::InputContext* input, uint64_t session, uint64_t revision, uint32_t index);

    fcitx::EventDispatcher dispatcher_;
    fcitx::Instance* instance_;
    std::unordered_map<fcitx::InputContext*, Context> contexts_;
    uint64_t next_ = 0;
    bool warnedLeftShift_ = false;
    Frame mode_;
    std::vector<std::unique_ptr<MenuAction>> commands_;
    fcitx::Menu menu_;
    fcitx::SimpleAction action_;
    SocketPipe pipe_; // Stop the worker before destroying its dispatcher.
};

} // namespace tango::ime
