# WoWBot - Project Description

In World of Warcraft 2v2 PVP Arenas, you try to destroy the opposing team in order to win.

This real-time chat app, incorporating a specialized PvP knowledgable agent, allows for oneself and a partner
to ask strategy questions before the match begins.

## Why this App and not just directly ChatGPT

The reasoning is purely logistical. 

In a game you only have 60 seconds before the fight begins, where you are provided the specialization/class information 
(what kind of fighters they are, e.g. "Arms Warrior") of the opposing duo. Using this limited knowledge, the agent, which
also possesses knowledge of the user duo, can assess the best strategy in the current PvP meta to quickly
explain how to maximize chances of winning. Information such as:

Who to go after first  
What strong abilities they have to watch out for  
How to leverage the map layout to your advantage  

ChatGPT could answer these questions standardly, but it would then require a read-out to the other player. Typically,
humans can read faster than they can speak, so it would be easier if both players shared a chat in which they
could individually read the output. Users will also be able to ask follow up questions which, again, both players can see. In this environment, with just 60 seconds to determine strategy, time is of the essence, and this is a faster way
to determine and understand as a team the optimal strategy for the match

# Learning Results

Working on this project has provided me with a greater understanding on some of the following concepts

Golang programming language  
Angular framework  
Websockets  
External API interaction  
OAuth  
AI agent prompting and memory  
Obtaining and leveraging tokens (from API into web browser storage)  

As a note, the original project was developed following a tutorial guide shown here:
https://www.thepolyglotdeveloper.com/2016/12/create-real-time-chat-app-golang-angular-2-websockets/
Further iteration on the code (with the help of ChatGPT) has resulted in this new, purposeful application