#include "tangobridge.h"

#include <fcitx/addonfactory.h>
#include <fcitx/addonmanager.h>
#include <fcitx/candidatelist.h>
#include <fcitx/event.h>
#include <fcitx/globalconfig.h>
#include <fcitx/inputpanel.h>
#include <fcitx/instance.h>
#include <fcitx/statusarea.h>
#include <fcitx/userinterfacemanager.h>
#include <algorithm>
#include <codecvt>
#include <cstdlib>
#include <cstdio>
#include <fstream>
#include <locale>
#include <stdexcept>
#include <unistd.h>

namespace tango::ime {
namespace {

struct Settings {
    std::string runtime, host, connection;
};

Settings settings() {
    Settings result;
    if (const char* runtime = std::getenv("XDG_RUNTIME_DIR"))
        result.runtime = runtime;
    std::ifstream file(TANGO_FCITX_CONFIG);
    std::string field, socket;
    while (std::getline(file, field)) {
        if (field.size() > 4096) break;
        if (field.compare(0, 7, "socket=") == 0) socket = field.substr(7);
        if (field.compare(0, 5, "host=") == 0) result.host = field.substr(5);
        if (field.compare(0, 11, "connection=") == 0) result.connection = field.substr(11);
    }
    if (result.runtime.empty() || result.runtime.front() != '/' || result.host.empty() ||
        result.host.front() != '/' || result.connection.empty() || result.connection.size() > 120 ||
        result.connection.find_first_not_of("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_.") !=
            std::string::npos || socket != "tango-ime/" + result.connection + ".sock")
        throw std::runtime_error("invalid Tango Fcitx5 configuration");
    result.runtime += "/" + socket;
    return result;
}

std::string toUTF8(const std::u16string& text) {
    return std::wstring_convert<std::codecvt_utf8_utf16<char16_t>, char16_t>().to_bytes(text);
}

std::u16string fromUTF8(const std::string& text) {
    return std::wstring_convert<std::codecvt_utf8_utf16<char16_t>, char16_t>().from_bytes(text);
}

std::string label(const Candidate& choice) {
    auto text = toUTF8(choice.text);
    if (!choice.comment.empty()) text += "  " + toUTF8(choice.comment);
    return text;
}

Frame keyFrame(const fcitx::KeyEvent& event, uint64_t session) {
    const auto key = event.rawKey();
    const auto state = key.states();
    Frame frame;
    frame.type = Type::test_key;
    frame.session = session;
    auto symbol = static_cast<uint32_t>(key.sym());
    frame.vk = symbol <= UINT16_MAX ? static_cast<uint16_t>(symbol) : 0;
    frame.scan = static_cast<uint16_t>(std::max(0, key.code() - 8));
    frame.flags = event.isRelease() ? KeyFlag::up : KeyFlag::down;
    if (state.test(fcitx::KeyState::Repeat)) frame.flags |= KeyFlag::repeat;
    if (state.test(fcitx::KeyState::Shift)) frame.modifiers |= Modifier::shift;
    if (state.test(fcitx::KeyState::Ctrl)) frame.modifiers |= Modifier::ctrl;
    if (state.test(fcitx::KeyState::Alt)) frame.modifiers |= Modifier::alt;
    if (state.test(fcitx::KeyState::Super) || state.test(fcitx::KeyState::Meta)) frame.modifiers |= Modifier::meta;
    if (state.test(fcitx::KeyState::CapsLock)) frame.modifiers |= Modifier::caps_lock;
    if (!event.isRelease()) frame.text = fromUTF8(fcitx::Key::keySymToUTF8(key.sym()));
    return frame;
}

} // namespace

class TangoBridge::Word final : public fcitx::CandidateWord {
public:
    Word(const Candidate& choice, TangoBridge& bridge, uint64_t session, uint64_t revision, uint32_t index)
        : CandidateWord(fcitx::Text(label(choice))), bridge_(bridge),
          session_(session), revision_(revision), index_(index) {}

    void select(fcitx::InputContext* input) const override {
        if (revision_) bridge_.selectCandidate(input, session_, revision_, index_);
    }

private:
    TangoBridge& bridge_;
    uint64_t session_;
    uint64_t revision_;
    uint32_t index_;
};

TangoBridge::TangoBridge(fcitx::Instance* instance) : instance_(instance) {
    const auto config = settings();
    action_.setShortText("Tango");
    action_.setIcon("input-keyboard");
    action_.setMenu(&menu_);
    instance_->userInterfaceManager().registerAction(TANGO_FCITX_ADDON, &action_);
    // The dispatcher is associated with the Fcitx main loop. Initialization
    // just starts a worker and never waits for either connection or host.
    dispatcher_.attach(&instance_->eventLoop());
    pipe_.start(config.runtime, config.host, [this] {
        dispatcher_.schedule([this] {
            try { receive(); }
            catch (...) {}
        });
    });
}

TangoBridge::Context& TangoBridge::context(fcitx::InputContext* input) {
    auto& value = contexts_[input];
    if (!value.view.get()) {
        value = {input->watch(), ++next_, 0};
        input->statusArea().addAction(fcitx::StatusGroup::InputMethod, &action_);
    }
    return value;
}

bool TangoBridge::open(Context& value) {
    if (!pipe_.connected()) return false;
    auto generation = pipe_.generation();
    if (value.opened == generation) return true;
    Frame frame;
    frame.type = Type::open;
    frame.session = value.session;
    if (!pipe_.post(frame)) return false;
    frame.type = Type::focus;
    frame.focused = true;
    if (!pipe_.post(frame)) return false;
    value.opened = generation;
    frame.type = Type::menu_open;
    pipe_.post(std::move(frame));
    return true;
}

void TangoBridge::activate(const fcitx::InputMethodEntry&, fcitx::InputContextEvent& event) {
    try {
        if (!warnedLeftShift_) {
            const auto& config = instance_->globalConfig();
            const auto leftShift = fcitx::Key("Shift_L");
            for (const auto* keys : {&config.altTriggerKeys(), &config.triggerKeys(),
                                    &config.activateKeys(), &config.deactivateKeys(),
                                    &config.enumerateForwardKeys(), &config.enumerateBackwardKeys(),
                                    &config.enumerateGroupForwardKeys(), &config.enumerateGroupBackwardKeys()}) {
                if (leftShift.checkKeyList(*keys)) {
                    std::fputs("Tango IME: standalone left Shift is reserved by Fcitx5. "
                               "Use right Shift or change the shortcut in Fcitx5 Configuration > Global Options > Hotkey.\n",
                               stderr);
                    warnedLeftShift_ = true;
                    break;
                }
            }
        }
        open(context(event.inputContext()));
    }
    catch (...) {}
}

void TangoBridge::deactivate(const fcitx::InputMethodEntry&, fcitx::InputContextEvent& event) {
    try {
        const auto found = contexts_.find(event.inputContext());
        if (found == contexts_.end()) return;
        const auto session = found->second.session;
        if (found->second.opened == pipe_.generation() && pipe_.connected()) {
            Frame focus;
            focus.type = Type::focus;
            focus.session = session;
            // Fcitx or its frontend commits client preedit on focus-out.
            // Panel preedit and engine switches still need the host's final edits.
            const auto& panel = event.inputContext()->inputPanel();
            if (event.type() != fcitx::EventType::InputContextFocusOut ||
                panel.preedit().textLength() != 0) {
                if (auto result = pipe_.request(focus, std::chrono::milliseconds(50))) {
                    receive(result->received_through);
                    for (auto& frame : result->edits) apply(std::move(frame));
                }
            }
            Frame close;
            close.type = Type::close;
            close.session = session;
            pipe_.post(std::move(close));
        }
        contexts_.erase(event.inputContext());
        clearPanel(*event.inputContext());
    } catch (...) {}
}

void TangoBridge::reset(const fcitx::InputMethodEntry&, fcitx::InputContextEvent& event) {
    try {
        auto found = contexts_.find(event.inputContext());
        if (found != contexts_.end() && found->second.opened == pipe_.generation()) {
            Frame frame;
            frame.type = Type::reset;
            frame.session = found->second.session;
            pipe_.post(std::move(frame));
        }
        clearPanel(*event.inputContext());
    } catch (...) {}
}

void TangoBridge::keyEvent(const fcitx::InputMethodEntry&, fcitx::KeyEvent& event) {
    try {
        auto& value = context(event.inputContext());
        if (!open(value)) return;
        auto outcome = pipe_.request(keyFrame(event, value.session));
        if (!outcome) return;
        receive(outcome->received_through);
        for (auto& frame : outcome->edits)
            apply(std::move(frame));
        if (outcome->eaten) event.filterAndAccept();
    } catch (...) {}
}

void TangoBridge::selectCandidate(fcitx::InputContext* input, uint64_t session, uint64_t revision, uint32_t index) {
    try {
        const auto found = contexts_.find(input);
        if (!input->hasFocus() || found == contexts_.end() ||
            found->second.session != session || found->second.opened != pipe_.generation())
            return;
        Frame pick;
        pick.type = Type::candidate_pick;
        pick.session = session;
        pick.candidate_revision = revision;
        pick.candidate_index = index;
        if (auto result = pipe_.request(std::move(pick))) {
            receive(result->received_through);
            for (auto& frame : result->edits) apply(std::move(frame));
        }
    } catch (...) {}
}

void TangoBridge::clearPanel(fcitx::InputContext& input) {
    auto& panel = input.inputPanel();
    panel.setClientPreedit({});
    panel.setPreedit({});
    panel.setCandidateList(nullptr);
    input.updatePreedit();
    input.updateUserInterface(fcitx::UserInterfaceComponent::InputPanel);
}

void TangoBridge::receive(uint64_t through) {
    if (!pipe_.connected()) {
        for (auto& entry : contexts_) {
            if (auto* input = entry.second.view.get(); input && entry.second.opened != 0)
                clearPanel(*input);
            entry.second.opened = 0;
        }
        return;
    }
    for (auto current = contexts_.begin(); current != contexts_.end();) {
        if (!current->second.view.get())
            current = contexts_.erase(current);
        else
            ++current;
    }
    for (auto& entry : contexts_) open(entry.second);
    for (auto& frame : pipe_.received(through))
        apply(std::move(frame));
}

void TangoBridge::apply(Frame frame) {
    if (frame.type == Type::mode) {
        mode_ = std::move(frame);
        action_.setShortText(toUTF8(mode_.short_text));
        action_.setLongText(toUTF8(mode_.label));
        if (std::getenv("TANGO_IME_SMOKE_TRACE"))
            std::fprintf(stderr, "Tango Fcitx5 mode %s\n", toUTF8(mode_.id).c_str());
        for (auto& entry : contexts_) {
            if (auto* context = entry.second.view.get()) action_.update(context);
        }
        return;
    }
    if (frame.type == Type::menu) {
        refreshMenu(frame.items);
        return;
    }
    for (const auto& entry : contexts_) {
        if (entry.second.session != frame.session) continue;
        auto* input = entry.second.view.get();
        if (!input) return;
        auto& panel = input->inputPanel();
        if (frame.type == Type::commit) {
            clearPanel(*input);
            input->commitString(toUTF8(frame.text));
        } else if (frame.type == Type::compose) {
            size_t cursor = 0;
            for (uint32_t rune = 0; rune < frame.cursor && cursor < frame.text.size(); ++rune) {
                if (frame.text[cursor] >= 0xd800 && frame.text[cursor] <= 0xdbff) ++cursor;
                ++cursor;
            }
            const auto byteOffset = toUTF8(frame.text.substr(0, cursor)).size();
            fcitx::Text text(toUTF8(frame.text), fcitx::TextFormatFlag::Underline);
            text.setCursor(static_cast<int>(byteOffset));
            if (input->capabilityFlags().test(fcitx::CapabilityFlag::Preedit)) {
                panel.setPreedit({});
                panel.setClientPreedit(text);
            } else {
                panel.setClientPreedit({});
                panel.setPreedit(text);
            }
            input->updatePreedit();
        } else if (frame.type == Type::candidates || frame.type == Type::candidate_list) {
            if (frame.candidates.empty()) panel.setCandidateList(nullptr);
            else {
                auto list = std::make_unique<fcitx::CommonCandidateList>();
                list->setPageSize(9);
                list->setLabels({"1", "2", "3", "4", "5", "6", "7", "8", "9"});
                for (uint32_t i = 0; i < frame.candidates.size(); ++i)
                    list->append(std::make_unique<Word>(frame.candidates[i], *this, frame.session,
                                                       frame.candidate_revision, i));
                list->setGlobalCursorIndex(frame.selected >= 0 &&
                    static_cast<size_t>(frame.selected) < frame.candidates.size() ? frame.selected : -1);
                panel.setCandidateList(std::move(list));
            }
        }
        input->updateUserInterface(fcitx::UserInterfaceComponent::InputPanel);
        if ((frame.type == Type::candidates || frame.type == Type::candidate_list) &&
            std::getenv("TANGO_IME_SMOKE_TRACE"))
            std::fprintf(stderr, "Tango Fcitx5 candidates %zu\n", frame.candidates.size());
        return;
    }
}

void TangoBridge::refreshMenu(const std::vector<MenuItem>& items) {
    for (const auto& item : commands_) menu_.removeAction(&item->action);
    commands_.clear();
    for (const auto& item : items) {
        auto command = std::make_unique<MenuAction>();
        command->action.setShortText(toUTF8(item.title));
        command->action.setChecked((item.flags & checked) != 0);
        if (item.flags & separator) command->action.setSeparator(true);
        if (!(item.flags & (disabled | separator))) {
            command->handler = command->action.connect<fcitx::SimpleAction::Activated>(
                [this, id = item.id](fcitx::InputContext*) {
                    try {
                        Frame pick;
                        pick.type = Type::menu_pick;
                        pick.id = id;
                        pipe_.post(std::move(pick));
                    } catch (...) {}
                });
        }
        menu_.addAction(&command->action);
        commands_.push_back(std::move(command));
    }
}

std::string TangoBridge::subModeLabelImpl(const fcitx::InputMethodEntry&, fcitx::InputContext&) {
    try { return toUTF8(mode_.short_text); }
    catch (...) { return {}; }
}

std::string TangoBridge::subModeIconImpl(const fcitx::InputMethodEntry&, fcitx::InputContext&) {
    try { return toUTF8(mode_.icon); }
    catch (...) { return {}; }
}

class Factory final : public fcitx::AddonFactory {
    fcitx::AddonInstance* create(fcitx::AddonManager* manager) override {
        try { return new TangoBridge(manager->instance()); }
        catch (...) { return nullptr; }
    }
};

} // namespace tango::ime

FCITX_ADDON_FACTORY(tango::ime::Factory);
