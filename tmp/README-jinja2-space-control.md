gonja2 and jinja2 maybe has problem with controlling space

jinja2 ahs two - block tag {% %} to start for, if, macro etc .. and the variable render tag {{ }} which can be customized.

go template only has one and shared for both purposes

Specs

trim_blocks: True means starting block char {%  %} - the space and newline after will be trimmed
lstrip_blocks: True means leading (on the left, or before ) white space and new line is stripped at the starting block char

When a for loop the starting for and the endfor always executed per each loop thus these settings are repeatedly applied for both lines

If nested if 

trim_blocks: True: Removes the first newline immediately following the {% else %} tag.
lstrip_blocks: True: Strips all leading tabs and spaces on the same line before the {% else %} tag. 

macro 

trim_blocks: True: Automatically removes the first newline following the {% macro ... %} and {% endmacro %} tags. This prevents an empty line from being rendered at the start or end of your macro's output.

lstrip_blocks: True: Strips all leading tabs and spaces on the line where the {% macro %} or {% endmacro %} tag is located, up until the start of the tag itself. This allows you to indent your macro definitions for readability without that indentation appearing in the final rendered text. 

When calling a macro using {{ my_macro() }} or using a {% call %} block:

trim_blocks: Only affects block tags (those starting with {% ), not variable tags like {{ ... }}. Therefore, it will remove the newline after a {% call %} or {% endcall %} tag, but it will not remove the newline after a standard macro call {{ my_macro() }}.

lstrip_blocks: Similarly strips leading whitespace before the {% call %} or {% endcall %} tags.

lstrip_blocks with if/else 
Action: Strips all leading tabs and spaces on the same line before an {% if %}, {% elif %}, {% else %}, or {% endif %} tag.
Result: This is crucial for nested if statements. You can indent your code to see the logic clearly (e.g., indenting an inner if inside an outer if), and Jinja will remove those indents so they don't mess up your final file's alignment. 


Gonja2 latest problems is in test1.j2 - there is no way to make it work - the + in if does not work.

Controlling indentaiton and space is *much* better with go template.